package legal

import (
	_ "embed"
	"fmt"
)

var (
	//go:embed notice.txt
	appNotice string

	//go:embed license.txt
	appLicence string
)

func ShowLicenses() {
	fmt.Println(appNotice)
	fmt.Println()
	fmt.Println(appLicence)
}
