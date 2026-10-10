package libro

import (
	r "github.com/michalCapo/g-sui/ui"
)

type actionAppNotifyInput struct {
	SID      string `json:"sid"`
	Subtitle string `json:"subtitle"`
	Title    string `json:"title"`
	Variant  string `json:"variant"`
}

type actionAppURLsInput struct {
	SID string `json:"sid"`
	ID  string `json:"id"`
}

type actionPluginOpenInput struct {
	Dock   string `json:"dock"`
	Plugin string `json:"plugin"`
	SID    string `json:"sid"`
}

type actionAppDockInput struct {
	Dock string `json:"dock"`
	ID   string `json:"id"`
	SID  string `json:"sid"`
}

var actionAppStart r.ActionRef[actionAppStartInput]

type actionAppStartInput struct {
	AddAgent          bool    `json:"addAgent"`
	AutolaunchProject *string `json:"autolaunchProject"`
	Command           string  `json:"command"`
	Dock              string  `json:"dock"`
	EditorFile        *string `json:"editorFile"`
	FilesPanel        string  `json:"filesPanel"`
	IconUrl           string  `json:"iconUrl"`
	Name              string  `json:"name"`
	Parents           float64 `json:"parents"`
	Plugin            string  `json:"plugin"`
	ReplaceAgent      bool    `json:"replaceAgent"`
	SID               string  `json:"sid"`
	Side              string  `json:"side"`
	Type              string  `json:"type"`
	Url               string  `json:"url"`
	Width             *string `json:"width"`
	Writable          *bool   `json:"writable"`
}

type actionAppHydrateInput struct {
	ID      string `json:"id"`
	OpenURL bool   `json:"openURL"`
	SID     string `json:"sid"`
}

var actionAppClose r.ActionRef[actionAppCloseInput]

type actionAppCloseInput struct {
	ID  string `json:"id"`
	SID string `json:"sid"`
}

type actionAppCloseOthersInput struct {
	ID  string `json:"id"`
	SID string `json:"sid"`
}

type actionAppTerminalRestartInput struct {
	ID  string `json:"id"`
	SID string `json:"sid"`
}

type actionAppMoveToProjectInput struct {
	Branch  string `json:"branch"`
	Kind    string `json:"kind"`
	Path    string `json:"path"`
	Project string `json:"project"`
	SID     string `json:"sid"`
	Target  string `json:"target"`
}

type actionAppResizeInput struct {
	ID       string   `json:"id"`
	MaxPixel *float64 `json:"maxPixel"`
	SID      string   `json:"sid"`
	Width    *string  `json:"width"`
}

type actionAppResizeMaxToggleInput struct {
	MaxPixel *float64 `json:"maxPixel"`
	SID      string   `json:"sid"`
}

type actionAppResizeStepInput struct {
	Delta    *float64 `json:"delta"`
	MaxPixel *float64 `json:"maxPixel"`
	SID      string   `json:"sid"`
}

var actionAppSelect r.ActionRef[actionAppSelectInput]

type actionAppSelectInput struct {
	Focus *bool    `json:"focus"`
	Index *float64 `json:"index"`
	SID   string   `json:"sid"`
}

type actionAppBrowseOpenInput struct {
	SID  string `json:"sid"`
	Side string `json:"side"`
}

type actionAppUrlSetInput struct {
	ID       string `json:"id"`
	Observed bool   `json:"observed"`
	SID      string `json:"sid"`
	Url      string `json:"url"`
}

type actionProjectCreateInput struct {
	ProjectPath string `json:"project-path"`
	SID         string `json:"sid"`
}

type actionProjectOpenFolderInput struct {
	ProjectPath string `json:"project-path"`
	SID         string `json:"sid"`
}

type actionProjectCreateConfirmInput struct {
	Path string `json:"path"`
	SID  string `json:"sid"`
}

var actionProjectClose r.ActionRef[actionProjectCloseInput]

type actionProjectCloseInput struct {
	Name string `json:"name"`
	SID  string `json:"sid"`
}

type actionProjectSwitchInput struct {
	AppId      string `json:"appId"`
	FocusAgent bool   `json:"focusAgent"`
	Name       string `json:"name"`
	SID        string `json:"sid"`
}

type actionProjectRemoveInput struct {
	Name string `json:"name"`
	SID  string `json:"sid"`
}

var actionAppCloseAll r.ActionRef[sessionInput]

type actionWorktreeSwitchInput struct {
	Branch  string `json:"branch"`
	Path    string `json:"path"`
	Project string `json:"project"`
	SID     string `json:"sid"`
}

type actionWorktreeCreateInput struct {
	Branch string `json:"branch"`
	SID    string `json:"sid"`
}

type actionProjectCommandAgentInput struct {
	Operation string `json:"operation"`
	Project   string `json:"project"`
	Request   string `json:"request"`
	SID       string `json:"sid"`
}

type actionChildrenControlInput struct {
	Command childCommand `json:"command"`
	Project string       `json:"project"`
	Request string       `json:"request"`
	SID     string       `json:"sid"`
}

type actionFilesInput struct {
	Column  float64 `json:"column"`
	ID      string  `json:"id"`
	Ignored bool    `json:"ignored"`
	Kind    string  `json:"kind"`
	Line    float64 `json:"line"`
	Parents float64 `json:"parents"`
	Path    string  `json:"path"`
	Query   string  `json:"query"`
	Request string  `json:"request"`
	SID     string  `json:"sid"`
	Version string  `json:"version"`
}

type actionSettingsToolKeysInput struct {
	Bindings map[string]string `json:"bindings"`
	SID      string            `json:"sid"`
}

type actionProjectCommandSaveInput struct {
	Command string `json:"command"`
	Mode    string `json:"mode"`
	Name    string `json:"name"`
	Port    string `json:"port"`
	SID     string `json:"sid"`
}

type actionSettingsAgentEnvironmentInput struct {
	Entries []environmentInput `json:"entries"`
	SID     string             `json:"sid"`
}

type actionSettingsThreadAgentInput struct {
	Agent *string `json:"agent"`
	SID   string  `json:"sid"`
}

type actionSettingsPageToolsInput struct {
	Autoexecute *bool  `json:"autoexecute"`
	SID         string `json:"sid"`
}

type actionSettingsVoiceInput struct {
	Key      string `json:"key"`
	ClearKey bool   `json:"clearKey"`
	SID      string `json:"sid"`
}

type actionSettingsAgentUpdatesInput struct {
	Enabled    bool `json:"enabled"`
	Initialize bool `json:"initialize"`
}

type actionSettingsAgentCommandInput struct {
	Commands map[string]string `json:"commands"`
	Custom   []Plugin          `json:"custom"`
	Disabled map[string]bool   `json:"disabled"`
	Names    map[string]string `json:"names"`
	Order    []string          `json:"order"`
	Removed  map[string]bool   `json:"removed"`
	SID      string            `json:"sid"`
}

type actionSettingsWidthInput struct {
	SID   string `json:"sid"`
	Width string `json:"width"`
}

var actionThreadCreate r.ActionRef[actionThreadCreateInput]

type actionThreadCreateInput struct {
	Agent   string `json:"agent"`
	Name    string `json:"name"`
	Project string `json:"project"`
	SID     string `json:"sid"`
}

type actionThreadRenameInput struct {
	AppID    string `json:"appId"`
	ID       string `json:"id"`
	Name     string `json:"name"`
	SID      string `json:"sid"`
	Fallback bool   `json:"fallback"`
}

type actionWorktreeTitleInput struct {
	AppID    string `json:"appId"`
	Name     string `json:"name"`
	Project  string `json:"project"`
	SID      string `json:"sid"`
	Fallback bool   `json:"fallback"`
}

type actionThreadArchiveInput struct {
	Archived bool   `json:"archived"`
	ID       string `json:"id"`
	SID      string `json:"sid"`
}

type actionSettingsToolsInput struct {
	Bindings map[string]string `json:"bindings"`
	Editor   *string           `json:"editor"`
	SID      string            `json:"sid"`
	Tools    []Plugin          `json:"tools"`
}

type actionThreadFinishPreviewInput struct {
	Base    string `json:"base"`
	Name    string `json:"name"`
	Request string `json:"request"`
	SID     string `json:"sid"`
}

type actionThreadFinishInput struct {
	Base       string `json:"base"`
	Body       string `json:"body"`
	Message    string `json:"message"`
	Method     string `json:"method"`
	Name       string `json:"name"`
	SID        string `json:"sid"`
	SourceHead string `json:"sourceHead"`
	TargetHead string `json:"targetHead"`
}

type sessionInput struct {
	SID string `json:"sid"`
}
