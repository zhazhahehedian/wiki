package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/zenith-wang/it-wiki/backend/internal/skillbuilder"
)

func (h *RegistryHandler) SkillTemplate(w http.ResponseWriter, r *http.Request) {
	user, ok := registryUser(w, r)
	if !ok {
		return
	}
	versionID := r.URL.Query().Get("version_id")
	if versionID == "" {
		WriteError(w, r, NewAPIError(400, "template_version_required", "请从市场详情选择具体的上线版本"))
		return
	}
	d, files, err := h.service.PublishedFiles(r.Context(), user, chi.URLParam(r, "slug"), versionID)
	if err != nil {
		registryHTTPError(w, r, err)
		return
	}
	draft, err := skillbuilder.Template(d, files)
	if err != nil {
		WriteError(w, r, NewAPIError(422, "skill_template_unsupported", "此 Skill 包含创建助手暂不支持的文件、额外文件头或内容大小，无法完整复制；请下载原包使用，或整理为文本 Skill 后上传"))
		return
	}
	WriteJSON(w, 200, map[string]any{"draft": draft, "source": map[string]string{"slug": d.Slug, "name": d.Name, "version_id": versionID, "version": d.Versions[0].Version}})
}
