package apiclient

type ValidationResponse struct {
	Valid  bool              `json:"valid"`
	Errors []ValidationError `json:"errors,omitempty"`
}

type TemplateValidateRequest struct {
	Platform string `json:"platform"`
	Job      string `json:"job"`
	Volumes  string `json:"volumes"`
}

type VolumeValidateRequest struct {
	Platform   string `json:"platform"`
	Definition string `json:"definition"`
}
