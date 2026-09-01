package main

import (
	"crypto/sha256"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"text/template"

	"github.com/paularlott/knot/build"
)

// The formula installs the CLI on PATH. On macOS the release zip contains a
// Knot.app bundle, so it is installed under libexec and the binary inside is
// symlinked into bin. The cask installs Knot.app into /Applications for the
// desktop experience (server + menu bar tray) and also links the CLI.
const formulaTemplate = `class Knot < Formula
	desc "A tool for creating and managing developer environments within a Nomad cluster"
	homepage "https://getknot.dev"
	license "Apache-2.0"
	version "{{ .Version }}"
  conflicts_with "knot-pro", because: "knot-pro is a commercial version of knot and cannot be installed alongside the open-source version"
	on_macos do
		on_arm do
			url "https://github.com/paularlott/knot/releases/download/v#{version}/knot_darwin_arm64.zip"
			sha256 "{{ .Checksum.DarwinArm64 }}"
		end
		on_intel do
			url "https://github.com/paularlott/knot/releases/download/v#{version}/knot_darwin_amd64.zip"
			sha256 "{{ .Checksum.DarwinAmd64 }}"
		end
	end

	on_linux do
		on_arm do
			url "https://github.com/paularlott/knot/releases/download/v#{version}/knot_linux_arm64.zip"
			sha256 "{{ .Checksum.LinuxArm64 }}"
		end
		on_intel do
			url "https://github.com/paularlott/knot/releases/download/v#{version}/knot_linux_amd64.zip"
			sha256 "{{ .Checksum.LinuxAmd64 }}"
		end
	end

	def install
		if OS.mac?
			# The cask also links the knot CLI into bin; refuse to fight over
			# the symlink. Homebrew has no formula<->cask conflicts_with DSL.
			if (HOMEBREW_PREFIX/"Caskroom/knot").directory?
				odie "knot cask is installed, which also provides the knot CLI. Uninstall it first:\n  brew uninstall --cask paularlott/tap/knot"
			end

			# macOS zip contains Knot.app — install under libexec, symlink the CLI.
			# Homebrew stages single-root archives from inside the root, so the
			# working directory is Knot.app itself and "Contents" is at its root.
			(libexec/"Knot.app").install "Contents"
			bin.install_symlink libexec/"Knot.app/Contents/MacOS/knot"
		else
			bin.install "knot"
		end
	end

	def caveats
		on_macos do
			<<~EOS
				For the desktop app with menu bar tray, install the cask instead:
				  brew install --cask paularlott/tap/knot
			EOS
		end
	end
end
`

const caskTemplate = `cask "knot" do
	version "{{ .Version }}"

	on_arm do
		sha256 "{{ .Checksum.DarwinArm64 }}"
		url "https://github.com/paularlott/knot/releases/download/v#{version}/knot_darwin_arm64.zip"
	end
	on_intel do
		sha256 "{{ .Checksum.DarwinAmd64 }}"
		url "https://github.com/paularlott/knot/releases/download/v#{version}/knot_darwin_amd64.zip"
	end

	name "Knot"
	desc "A tool for creating and managing developer environments within a Nomad cluster"
	homepage "https://getknot.dev"

	app "Knot.app"

	# Also make the CLI available on PATH so the knot command works from the
	# terminal without separately installing the formula.
	postflight do
		# The formula also links the knot CLI; refuse to fight over the symlink.
		if File.directory?("#{HOMEBREW_PREFIX}/Cellar/knot")
			raise "knot formula is installed, which also provides the knot CLI. Uninstall it first:\n  brew uninstall knot"
		end

		# The app is ad-hoc signed (not notarized) and brew quarantines cask
		# downloads, which makes Gatekeeper kill the binary on first exec.
		# Strip the flag so the app and the CLI link work immediately.
		# Non-bang system_command: xattr -d fails if the attribute is absent.
		system_command "/usr/bin/xattr", args: ["-dr", "com.apple.quarantine", "/Applications/Knot.app"]

		FileUtils.ln_sf("/Applications/Knot.app/Contents/MacOS/knot", "#{HOMEBREW_PREFIX}/bin/knot")
	end

	uninstall_postflight do
		FileUtils.rm_f "#{HOMEBREW_PREFIX}/bin/knot"
	end

	zap trash: [
		"~/.config/knot",
		"~/.knot.toml",
	]
end
`

func main() {
	outDir := flag.String("out", "../homebrew-tap", "Directory of the homebrew tap repository to write into")
	flag.Parse()

	data := struct {
		Version  string
		Checksum struct {
			DarwinArm64 string
			DarwinAmd64 string
			LinuxArm64  string
			LinuxAmd64  string
		}
	}{
		Version: build.Version,
	}

	// Calculate the SHA256 checksums
	files := map[string]*string{
		"bin/knot_darwin_amd64.zip": &data.Checksum.DarwinAmd64,
		"bin/knot_darwin_arm64.zip": &data.Checksum.DarwinArm64,
		"bin/knot_linux_amd64.zip":  &data.Checksum.LinuxAmd64,
		"bin/knot_linux_arm64.zip":  &data.Checksum.LinuxArm64,
	}

	for file, checksum := range files {
		f, err := os.Open(file)
		if err != nil {
			fmt.Printf("Error opening file %s: %v\n", file, err)
			os.Exit(1)
		}

		h := sha256.New()
		if _, err := io.Copy(h, f); err != nil {
			fmt.Printf("Error calculating checksum for file %s: %v\n", file, err)
			f.Close()
			os.Exit(1)
		}

		*checksum = fmt.Sprintf("%x", h.Sum(nil))

		f.Close()
	}

	outputs := []struct {
		name     string
		template string
	}{
		{"Formula/knot.rb", formulaTemplate},
		{"Casks/knot.rb", caskTemplate},
	}

	for _, output := range outputs {
		tmpl, err := template.New("tmpl").Parse(output.template)
		if err != nil {
			fmt.Println("Error creating template:", err)
			os.Exit(1)
		}

		path := filepath.Join(*outDir, output.name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			fmt.Printf("Error creating directory for %s: %v\n", path, err)
			os.Exit(1)
		}

		f, err := os.Create(path)
		if err != nil {
			fmt.Printf("Error creating file %s: %v\n", path, err)
			os.Exit(1)
		}

		if err := tmpl.Execute(f, data); err != nil {
			fmt.Printf("Error executing template for %s: %v\n", path, err)
			f.Close()
			os.Exit(1)
		}
		f.Close()

		fmt.Println("Wrote", path)
	}
}
