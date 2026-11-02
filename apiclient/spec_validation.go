package apiclient

type ValidationResponse struct {
	Valid  bool              `json:"valid" msgpack:"valid"`
	Errors []ValidationError `json:"errors,omitempty" msgpack:"errors,omitempty"`
}

type TemplateValidateRequest struct {
	Platform string `json:"platform" msgpack:"platform"`
	Job      string `json:"job" msgpack:"job"`
	Volumes  string `json:"volumes" msgpack:"volumes"`
}

type VolumeValidateRequest struct {
	Platform   string `json:"platform" msgpack:"platform"`
	Definition string `json:"definition" msgpack:"definition"`
}
