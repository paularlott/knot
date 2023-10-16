package legal

import (
	"embed"
)

var (
  //go:embed license.txt
  AppLicence string

  //go:embed notice.txt
  AppNotice string

  //go:embed licenses/*
  LicenseFiles embed.FS
)
