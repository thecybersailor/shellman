package localapi

import (
	"shellman/cli/internal/fsbrowser"
	"shellman/cli/internal/global"
	"shellman/cli/internal/projectstate"
)

// @title Shellman LocalAPI
// @version 1.0
// @description Local API schema for TypeScript SDK generation.
// @BasePath /
type swaggerMeta struct{}

type SwaggerTypeSurface struct {
	ActiveProject     global.ActiveProject       `json:"active_project"`
	AppProgramsConfig global.AppProgramsConfig   `json:"app_programs_config"`
	GlobalConfig      global.GlobalConfig        `json:"global_config"`
	TaskTree          projectstate.TaskTree      `json:"task_tree"`
	TaskNode          projectstate.TaskNode      `json:"task_node"`
	PaneBinding       projectstate.PaneBinding   `json:"pane_binding"`
	PaneSnapshot      projectstate.PaneSnapshot  `json:"pane_snapshot"`
	TaskNoteRecord    projectstate.TaskNoteRecord `json:"task_note_record"`
	TaskMessageRecord projectstate.TaskMessageRecord `json:"task_message_record"`
	PaneRuntimeRecord projectstate.PaneRuntimeRecord `json:"pane_runtime_record"`
	TaskRuntimeRecord projectstate.TaskRuntimeRecord `json:"task_runtime_record"`
	FSItem            fsbrowser.Item             `json:"fs_item"`
	FSListResult      fsbrowser.ListResult       `json:"fs_list_result"`
}

// swaggerTypeSurface godoc
// @Summary Swagger type surface
// @Description Documentation-only contract endpoint for SDK generation.
// @Tags schema
// @Produce json
// @Success 200 {object} SwaggerTypeSurface
// @Router /_schema/types [get]
func swaggerTypeSurface() {}

