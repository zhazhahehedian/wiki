"use client";

import { useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useRouter } from "next/navigation";
import {
  actOnCapability,
  type CapabilityDetail,
  type GovernanceAction,
} from "@/lib/api/registry";
import {
  Field,
  fieldClass,
  primaryClass,
  secondaryClass,
  RegistryError,
} from "./registry-shared";

export const actionLabels: Record<GovernanceAction, string> = {
  submit: "提交审核",
  approve: "通过并上线",
  reject: "驳回",
  offline: "下线能力",
  "new-draft": "发布新版本",
  "force-publish": "强制上线",
  visibility: "覆盖上线可见范围",
};
export function GovernanceControls({ item }: { item: CapabilityDetail }) {
  const client = useQueryClient();
  const router = useRouter();
  const [action, setAction] = useState<GovernanceAction>();
  const [reason, setReason] = useState("");
  const [visibility, setVisibility] = useState(
    item.published?.visibility ?? item.visibility,
  );
  const [department, setDepartment] = useState(
    item.published?.department ?? item.department,
  );
  const [allowlist, setAllowlist] = useState(
    (item.published?.allowlist ?? item.allowlist ?? []).join("\n"),
  );
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>();
  const candidate =
    item.draft_version_id ||
    (item.status === "offline" ? item.current_version_id : undefined);
  const actions: GovernanceAction[] = [];
  if (item.is_owner && ["draft", "offline"].includes(item.status) && candidate)
    actions.push("submit");
  if (item.is_owner && ["published", "offline"].includes(item.status))
    actions.push("new-draft");
  if (item.is_admin && item.status === "in_review")
    actions.push("approve", "reject");
  if ((item.is_admin || item.is_owner) && item.is_live) actions.push("offline");
  if (
    item.is_admin &&
    ["draft", "in_review", "offline"].includes(item.status) &&
    candidate
  )
    actions.push("force-publish");
  if (item.is_admin && item.is_live && item.status !== "in_review")
    actions.push("visibility");
  if (!item.is_admin && !item.is_owner) return null;
  async function execute() {
    if (!action) return;
    const version = ["offline", "new-draft", "visibility"].includes(action)
      ? item.current_version_id
      : candidate;
    if (!version) return;
    setBusy(true);
    setError(undefined);
    try {
      const result = await actOnCapability(item.slug, action, {
        revision: item.revision,
        version_id: version,
        reason,
        ...(action === "visibility"
          ? {
              visibility,
              department,
              allowlist:
                visibility === "allowlist"
                  ? allowlist.split(/\s+/).filter(Boolean)
                  : [],
            }
          : {}),
      });
      client.setQueryData(["registry", "detail", item.slug], result);
      await client.invalidateQueries({ queryKey: ["registry"] });
      setAction(undefined);
      setReason("");
      if (action === "new-draft")
        router.push(`/hub/registry/${item.slug}/edit`);
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  }
  const needsReason =
    action &&
    ["reject", "offline", "force-publish", "visibility"].includes(action);
  return (
    <section className="space-y-4 rounded-xl border border-zinc-100 bg-zinc-50/50 p-5">
      <div className="text-sm leading-6 text-zinc-600">
        {item.is_live
          ? "当前上线版本继续对授权用户可见，新版本经审核后替换。"
          : "当前能力未上线，消费者无法访问。"}
        {item.status === "in_review" && " 待审内容已锁定。"}
      </div>
      {item.review_reason && (
        <p className="rounded-lg bg-amber-50 p-3 text-sm text-amber-900">
          审核/下线说明：{item.review_reason}
        </p>
      )}
      <div className="flex flex-wrap gap-2">
        {actions.map((a) => (
          <button
            key={a}
            disabled={busy}
            className={
              a === "submit" || a === "approve" ? primaryClass : secondaryClass
            }
            onClick={() => {
              setAction(a);
              setError(undefined);
            }}
          >
            {actionLabels[a]}
          </button>
        ))}
      </div>
      {action && (
        <form
          aria-label={actionLabels[action]}
          className="space-y-4 border-t border-zinc-100 pt-4"
          onSubmit={(e) => {
            e.preventDefault();
            void execute();
          }}
        >
          <p className="text-sm font-medium">
            {actionLabels[action]} · {item.slug}
          </p>
          <p className="text-sm text-zinc-600">
            操作版本：
            {item.versions.find(
              (v) =>
                v.id ===
                (["offline", "new-draft", "visibility"].includes(action)
                  ? item.current_version_id
                  : candidate),
            )?.version ?? "当前候选"}
          </p>
          {action === "new-draft" && (
            <p className="text-sm text-zinc-500">
              新建可编辑草稿；请填写一个尚未发布的版本号。
              {item.draft_version_id && "下线时保留的候选将转为历史版本。"}
            </p>
          )}
          {action === "visibility" && (
            <>
              <Field label="生效可见范围">
                <select
                  className={fieldClass}
                  value={visibility}
                  onChange={(e) =>
                    setVisibility(e.target.value as typeof visibility)
                  }
                >
                  <option value="org">全公司</option>
                  <option value="department">同部门</option>
                  <option value="allowlist">指定名单</option>
                </select>
              </Field>
              <Field label="生效部门">
                <input
                  className={fieldClass}
                  maxLength={120}
                  required={visibility === "department"}
                  value={department}
                  onChange={(e) => setDepartment(e.target.value)}
                />
              </Field>
              {visibility === "allowlist" && (
                <Field label="生效名单 Open ID">
                  <textarea
                    className={fieldClass}
                    required
                    value={allowlist}
                    onChange={(e) => setAllowlist(e.target.value)}
                  />
                </Field>
              )}
            </>
          )}
          <Field label={`操作说明${needsReason ? "（必填）" : "（可选）"}`}>
            <textarea
              className={fieldClass}
              required={!!needsReason}
              maxLength={2000}
              value={reason}
              onChange={(e) => setReason(e.target.value)}
              placeholder="简述审核意见或操作原因"
            />
          </Field>
          {error != null && (
            <RegistryError
              error={error}
              retry={() => {
                void client.invalidateQueries({
                  queryKey: ["registry", "detail", item.slug],
                });
                setAction(undefined);
              }}
            />
          )}
          <div className="flex gap-2">
            <button
              className={primaryClass}
              disabled={busy || (!!needsReason && !reason.trim())}
            >
              {busy ? "正在处理…" : `确认${actionLabels[action]}`}
            </button>
            <button
              type="button"
              className={secondaryClass}
              disabled={busy}
              onClick={() => setAction(undefined)}
            >
              取消
            </button>
          </div>
        </form>
      )}
    </section>
  );
}
