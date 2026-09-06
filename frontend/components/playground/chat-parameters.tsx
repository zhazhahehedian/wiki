"use client";

import { useEffect, useState } from "react";
import { Popover } from "@base-ui/react/popover";
import { SlidersHorizontal, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import type { ChatParameters, ModelProtocol } from "@/lib/api/playground";

const fields = [
  {
    key: "temperature",
    label: "温度",
    hint: "较低偏稳定，较高偏多样。",
    min: 0,
    max: 2,
    step: 0.1,
    initial: 0.7,
  },
  {
    key: "top_p",
    label: "Top P",
    hint: "限定候选词范围，通常与温度择一调整。",
    min: 0,
    max: 1,
    step: 0.05,
    initial: 1,
  },
  {
    key: "frequency_penalty",
    label: "频率惩罚",
    hint: "正值降低重复用词的频率。",
    min: -2,
    max: 2,
    step: 0.1,
    initial: 0,
  },
  {
    key: "presence_penalty",
    label: "存在惩罚",
    hint: "正值鼓励引入新的话题。",
    min: -2,
    max: 2,
    step: 0.1,
    initial: 0,
  },
  {
    key: "max_tokens",
    label: "最大 Tokens",
    hint: "限制这次回复的输出长度。",
    min: 1,
    max: 32768,
    step: 1,
    initial: 4096,
  },
] as const;

// Protocol changes must never carry OpenAI-only fields to Claude.
export function parametersForProtocol(
  parameters: ChatParameters,
  protocol: ModelProtocol,
): ChatParameters {
  if (protocol !== "anthropic") return parameters;
  return {
    ...(parameters.temperature !== undefined
      ? { temperature: Math.min(parameters.temperature, 1) }
      : {}),
    ...(parameters.top_p !== undefined && parameters.temperature === undefined
      ? { top_p: parameters.top_p }
      : {}),
    ...(parameters.max_tokens !== undefined
      ? { max_tokens: parameters.max_tokens }
      : {}),
  };
}

export function ChatParameterSettings({
  parameters,
  onChange,
  systemPrompt,
  onSystemPromptChange,
  protocol,
  disabled,
}: {
  parameters: ChatParameters;
  onChange: (value: ChatParameters) => void;
  systemPrompt: string;
  onSystemPromptChange: (value: string) => void;
  protocol: ModelProtocol;
  disabled: boolean;
}) {
  const [open, setOpen] = useState(false);
  const [draftValues, setDraftValues] = useState<ChatParameters>({});
  const active = parametersForProtocol(parameters, protocol);
  const count = Object.keys(active).length + (systemPrompt.trim() ? 1 : 0);
  function setParameter(key: keyof ChatParameters, value: number | undefined) {
    const next = { ...active };
    if (value === undefined) delete next[key];
    else {
      next[key] = value;
      setDraftValues((current) => ({ ...current, [key]: value }));
      if (key === "temperature") delete next.top_p;
      if (key === "top_p") delete next.temperature;
    }
    onChange(next);
  }
  return (
    <Popover.Root open={open} onOpenChange={setOpen}>
      <Popover.Trigger
        render={<Button variant="ghost" size="sm" disabled={disabled} />}
        aria-label="模型参数"
      >
        <SlidersHorizontal className="size-4" />
        参数
        {count > 0 && (
          <span className="rounded bg-zinc-200 px-1.5 text-[10px] tabular-nums">
            {count}
          </span>
        )}
      </Popover.Trigger>
      <Popover.Portal>
        <Popover.Positioner
          side="top"
          align="start"
          sideOffset={12}
          collisionPadding={16}
          className="z-50"
        >
          <Popover.Popup
            aria-label="模型参数面板"
            className="w-80 max-w-[calc(100vw-2rem)] rounded-xl border border-zinc-200 bg-white p-4 text-zinc-900 shadow-lg outline-none"
          >
            <div className="flex items-start justify-between gap-2">
              <div>
                <Popover.Title className="text-sm font-semibold">
                  参数设置
                </Popover.Title>
                <Popover.Description className="mt-1 text-xs leading-5 text-zinc-500">
                  仅发送已启用的参数，支持范围以模型为准。
                </Popover.Description>
              </div>
              <Popover.Close
                render={<Button variant="ghost" size="icon-sm" />}
                aria-label="关闭参数"
              >
                <X className="size-4" />
              </Popover.Close>
            </div>
            <fieldset
              disabled={disabled}
              className="mt-3 max-h-[min(55dvh,440px)] space-y-4 overflow-y-auto pr-1"
            >
              {fields
                .filter(
                  (field) =>
                    protocol !== "anthropic" ||
                    !["frequency_penalty", "presence_penalty"].includes(
                      field.key,
                    ),
                )
                .map((field) => {
                  const enabled = active[field.key] !== undefined;
                  const max =
                    field.key === "temperature" && protocol === "anthropic"
                      ? 1
                      : field.max;
                  const value = Math.min(
                    active[field.key] ??
                      draftValues[field.key] ??
                      field.initial,
                    max,
                  );
                  return (
                    <div
                      key={field.key}
                      className="space-y-2 border-b border-zinc-100 pb-4"
                    >
                      <div className="flex items-center justify-between gap-3">
                        <label
                          htmlFor={`parameter-${field.key}`}
                          className="text-xs font-medium"
                        >
                          {field.label}
                        </label>
                        <input
                          type="checkbox"
                          role="switch"
                          aria-label={`启用${field.label}`}
                          checked={enabled}
                          onChange={(event) =>
                            setParameter(
                              field.key,
                              event.target.checked ? value : undefined,
                            )
                          }
                          className="relative h-4 w-8 shrink-0 cursor-pointer appearance-none rounded-full bg-zinc-200 transition-colors before:absolute before:left-0.5 before:top-0.5 before:size-3 before:rounded-full before:bg-white before:shadow-sm before:transition-transform checked:bg-primary checked:before:translate-x-4 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-primary disabled:cursor-default disabled:opacity-50"
                        />
                      </div>
                      <p className="text-[11px] leading-5 text-zinc-500">
                        {field.hint}
                      </p>
                      <div className="flex items-center gap-3">
                        {field.key !== "max_tokens" && (
                          <input
                            type="range"
                            aria-label={`${field.label}滑块`}
                            min={field.min}
                            max={max}
                            step={field.step}
                            value={value}
                            disabled={!enabled}
                            onChange={(event) =>
                              setParameter(
                                field.key,
                                Number(event.target.value),
                              )
                            }
                            className="min-w-0 flex-1 accent-primary disabled:opacity-40"
                          />
                        )}
                        <ParameterNumberInput
                          id={`parameter-${field.key}`}
                          min={field.min}
                          max={max}
                          step={field.step}
                          value={value}
                          disabled={!enabled}
                          onChange={(next) => setParameter(field.key, next)}
                        />
                      </div>
                    </div>
                  );
                })}
              {protocol === "anthropic" && (
                <p className="text-[11px] leading-5 text-zinc-500">
                  Claude 不发送惩罚参数；未启用最大 Tokens 时使用
                  4096。部分模型不接受采样参数，遇到参数错误可关闭温度与 Top P。
                </p>
              )}
              <div className="space-y-2">
                <label htmlFor="system-prompt" className="text-xs font-medium">
                  系统提示词
                </label>
                <Textarea
                  id="system-prompt"
                  rows={3}
                  value={systemPrompt}
                  onChange={(event) => onSystemPromptChange(event.target.value)}
                  placeholder="模型的角色、语气或回答要求…"
                />
                <p className="text-[11px] text-zinc-500">
                  留空使用模型默认行为，下次发送时生效。
                </p>
              </div>
            </fieldset>
            <Button
              variant="ghost"
              size="sm"
              className="mt-3 w-full"
              disabled={disabled}
              onClick={() => {
                onChange({});
                setDraftValues({});
                onSystemPromptChange("");
              }}
            >
              恢复默认参数
            </Button>
          </Popover.Popup>
        </Popover.Positioner>
      </Popover.Portal>
    </Popover.Root>
  );
}

function ParameterNumberInput({
  id,
  min,
  max,
  step,
  value,
  disabled,
  onChange,
}: {
  id: string;
  min: number;
  max: number;
  step: number;
  value: number;
  disabled: boolean;
  onChange: (value: number) => void;
}) {
  const [text, setText] = useState(String(value));
  useEffect(() => setText(String(value)), [value, disabled]);
  const number = Number(text);
  const valid =
    text.trim() !== "" &&
    Number.isFinite(number) &&
    number >= min &&
    number <= max &&
    (step !== 1 || Number.isInteger(number));
  return (
    <input
      id={id}
      type="number"
      min={min}
      max={max}
      step={step}
      value={text}
      disabled={disabled}
      aria-invalid={!disabled && !valid}
      onChange={(event) => {
        const next = event.target.valueAsNumber;
        setText(event.target.value);
        if (
          Number.isFinite(next) &&
          next >= min &&
          next <= max &&
          (step !== 1 || Number.isInteger(next))
        )
          onChange(next);
      }}
      onBlur={() => setText(String(value))}
      className="w-20 rounded-md border border-zinc-200 px-2 py-1 text-xs tabular-nums disabled:bg-zinc-50 disabled:text-zinc-400 aria-invalid:border-red-400"
    />
  );
}
