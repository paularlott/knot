package configwizard

import (
	"strings"
	"testing"
)

func TestDesktopFormDisablesFailedLoginBlocking(t *testing.T) {
	if !DefaultForm().AuthIPRateLimiting {
		t.Error("DefaultForm: failed-login blocking should default to enabled")
	}
	if DesktopForm().AuthIPRateLimiting {
		t.Error("DesktopForm: failed-login blocking should default to disabled on a single-user desktop")
	}
}

func TestAuthRateLimitCheckboxRenders(t *testing.T) {
	render := func(form Form) string {
		tmpl, err := htmlTemplate(wizardHTML)
		if err != nil {
			t.Fatal(err)
		}
		var b strings.Builder
		if err := tmpl.Execute(&b, wizardView{Form: form, BasePath: "/"}); err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(b.String(), "\n") {
			if strings.Contains(line, `id="auth_ip_rate_limiting"`) {
				return strings.TrimSpace(line)
			}
		}
		t.Fatal("auth_ip_rate_limiting checkbox not found in rendered page")
		return ""
	}

	if line := render(DefaultForm()); !strings.Contains(line, "checked>") {
		t.Errorf("default form should render the blocking toggle checked, got: %s", line)
	}
	if line := render(DesktopForm()); strings.Contains(line, "checked") {
		t.Errorf("desktop form should render the blocking toggle unchecked, got: %s", line)
	}
	if line := render(DesktopForm()); strings.Contains(line, "ZgotmplZ") {
		t.Errorf("desktop form rendered a filtered junk attribute, got: %s", line)
	}
}
