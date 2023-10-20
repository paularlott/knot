package legal

import (
	"embed"
	"fmt"
	"io/fs"
	"strings"
)

var (
  //go:embed license.txt
  appLicence string

  //go:embed notice.txt
  appNotice string

  //go:embed licenses/*
  licenseFiles embed.FS
)

func ShowLicenses() {

  // Show our notices
  fmt.Println(appLicence)
  sep()
  fmt.Println(appNotice)
  sep()

  // Walk from the root of licenseFiles getting the file name of all files in all directories
  fs.WalkDir(licenseFiles, ".", func(path string, d fs.DirEntry, err error) error {
    if err != nil {
      return err
    }

    if !d.IsDir() {
      fmt.Println(path[9:len(path)-len(path[strings.LastIndex(path, "/"):])] + "\n")

      data, err := licenseFiles.ReadFile(path)
      if err == nil {
        fmt.Println(string(data))
      }

      sep()
    }

    return nil
  })
}

func sep() {
  fmt.Println("\n------------------------------------------------------------")
  fmt.Print("\n")
}
