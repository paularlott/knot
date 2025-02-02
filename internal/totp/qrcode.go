package totp

import (
	"fmt"
	"io"
	"net/http"

	"github.com/yeqown/go-qrcode/v2"
	"github.com/yeqown/go-qrcode/writer/standard"
)

func ServeCreateQRCode(w http.ResponseWriter, code string, issuer string) error {

	// Construct OTP URL
	otpURL := "otpauth://totp/" + issuer + "?secret=" + code

	// Generate QR code
	qrc, err := qrcode.New(otpURL)
	if err != nil {
		return fmt.Errorf("Failed to generate QR code: %w", err)
	}

	writer := standard.NewWithWriter(struct {
		http.ResponseWriter
		io.Closer
	}{
		ResponseWriter: w,
		Closer:         io.NopCloser(nil),
	})

	err = qrc.Save(writer)
	if err != nil {
		return fmt.Errorf("Failed to write QR code: %w", err)
	}

	w.Header().Set("Content-Type", "image/jpeg")

	return nil
}
