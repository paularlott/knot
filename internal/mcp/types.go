package mcp

type SpaceInfo struct {
	SpaceID     string `json:"space_id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	IsDeployed  bool   `json:"is_deployed"`
	IsPending   bool   `json:"is_pending"`
	IsDeleting  bool   `json:"is_deleting"`
	Zone        string `json:"zone"`
	UserID      string `json:"user_id"`
	Username    string `json:"username"`
}
