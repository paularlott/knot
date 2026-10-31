package passkey

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/passkey/virtualauth"
)

const testOrigin = "https://knot.example.com"

var testDBPath string

func setup(t *testing.T) {
	t.Helper()
	// The database is opened once per process, so every test shares it.
	if testDBPath == "" {
		testDBPath = t.TempDir()
	}
	config.SetServerConfig(&config.ServerConfig{
		URL:           testOrigin,
		EncryptionKey: "test-key",
		BadgerDB:      config.BadgerDBConfig{Enabled: true, Path: testDBPath},
	})
}

func newUser(t *testing.T, name string) *model.User {
	t.Helper()
	u := model.NewUser(name, name+"@example.com", "password", nil, nil, "", "", "UTC", 0, "", 0, 0, 0)
	if err := database.GetInstance().SaveUser(u, nil); err != nil {
		t.Fatalf("SaveUser: %v", err)
	}
	return u
}

// browser carries the ceremony cookie from one request to the next.
type browser struct{ cookies []*http.Cookie }

func (b *browser) request() *http.Request {
	r := httptest.NewRequest(http.MethodPost, testOrigin+"/api", nil)
	for _, c := range b.cookies {
		r.AddCookie(c)
	}
	return r
}

func (b *browser) keep(w *httptest.ResponseRecorder) {
	for _, c := range w.Result().Cookies() {
		if c.MaxAge >= 0 {
			b.cookies = append(b.cookies, c)
		}
	}
}

func register(t *testing.T, b *browser, a *virtualauth.Authenticator, u *model.User) *model.Passkey {
	t.Helper()
	w := httptest.NewRecorder()
	creation, err := BeginRegistration(w, b.request(), u)
	if err != nil {
		t.Fatalf("BeginRegistration: %v", err)
	}
	b.keep(w)
	opts, _ := json.Marshal(creation)
	resp, _, err := a.Create(opts)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	pk, err := FinishRegistration(httptest.NewRecorder(), b.request(), u, resp, "  My key  ")
	if err != nil {
		t.Fatalf("FinishRegistration: %v", err)
	}
	if err := database.GetInstance().SaveUser(u, []string{"Passkeys"}); err != nil {
		t.Fatalf("SaveUser: %v", err)
	}
	return pk
}

// beginLogin returns the options and the browser holding the ceremony.
func beginLogin(t *testing.T) ([]byte, *browser) {
	t.Helper()
	b := &browser{}
	w := httptest.NewRecorder()
	assertion, err := BeginLogin(w, b.request())
	if err != nil {
		t.Fatalf("BeginLogin: %v", err)
	}
	b.keep(w)
	opts, _ := json.Marshal(assertion)
	return opts, b
}

func TestRegisterAndSignIn(t *testing.T) {
	setup(t)
	u := newUser(t, "pk-signin")
	a := virtualauth.New(testOrigin)
	pk := register(t, &browser{}, a, u)
	if pk.Name != "My key" {
		t.Errorf("name = %q, want trimmed", pk.Name)
	}

	opts, b := beginLogin(t)
	resp, _, err := a.Get(opts)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	res, err := FinishLogin(httptest.NewRecorder(), b.request(), resp)
	if err != nil {
		t.Fatalf("FinishLogin: %v", err)
	}
	if res.User.Id != u.Id || res.Passkey.Id != pk.Id || res.Passkey.SignCount != 1 || res.Passkey.LastUsedAt == nil {
		t.Errorf("result = user %s passkey %+v", res.User.Id, res.Passkey)
	}

	// The same ceremony can't be finished twice.
	if _, err := FinishLogin(httptest.NewRecorder(), b.request(), resp); !errors.Is(err, ErrChallenge) {
		t.Errorf("replay: err = %v, want ErrChallenge", err)
	}
}

func TestSignInRefusals(t *testing.T) {
	setup(t)
	u := newUser(t, "pk-refusals")
	a := virtualauth.New(testOrigin)
	register(t, &browser{}, a, u)

	t.Run("wrong origin", func(t *testing.T) {
		opts, b := beginLogin(t)
		resp, err := a.GetWith(opts, a.Creds[0], a.Creds[0].Counter+1, "https://evil.example.com")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := FinishLogin(httptest.NewRecorder(), b.request(), resp); !errors.Is(err, ErrVerify) {
			t.Errorf("err = %v, want ErrVerify", err)
		}
	})

	t.Run("counter regression", func(t *testing.T) {
		// Sign in once so the stored counter moves on.
		opts, b := beginLogin(t)
		resp, _, _ := a.Get(opts)
		res, err := FinishLogin(httptest.NewRecorder(), b.request(), resp)
		if err != nil {
			t.Fatal(err)
		}
		database.GetInstance().SaveUser(res.User, []string{"Passkeys"})

		opts, b = beginLogin(t)
		resp, err = a.GetWith(opts, a.Creds[0], 0, testOrigin)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := FinishLogin(httptest.NewRecorder(), b.request(), resp); !errors.Is(err, ErrCloneDetected) {
			t.Errorf("err = %v, want ErrCloneDetected", err)
		}
	})

	t.Run("inactive user", func(t *testing.T) {
		u.Active = false
		database.GetInstance().SaveUser(u, []string{"Active"})
		defer func() {
			u.Active = true
			database.GetInstance().SaveUser(u, []string{"Active"})
		}()
		opts, b := beginLogin(t)
		resp, _, _ := a.Get(opts)
		if _, err := FinishLogin(httptest.NewRecorder(), b.request(), resp); !errors.Is(err, ErrInactive) {
			t.Errorf("err = %v, want ErrInactive", err)
		}
	})

	t.Run("tampered cookie", func(t *testing.T) {
		opts, b := beginLogin(t)
		resp, _, _ := a.Get(opts)
		b.cookies[0].Value = "x" + b.cookies[0].Value
		if _, err := FinishLogin(httptest.NewRecorder(), b.request(), resp); !errors.Is(err, ErrChallenge) {
			t.Errorf("err = %v, want ErrChallenge", err)
		}
	})

	t.Run("removed passkey", func(t *testing.T) {
		other := newUser(t, "pk-removed")
		oa := virtualauth.New(testOrigin)
		pk := register(t, &browser{}, oa, other)
		if _, ok := Remove(other, pk.Id); !ok || len(other.Passkeys) != 0 {
			t.Fatalf("Remove: ok=%v left=%d", ok, len(other.Passkeys))
		}
		database.GetInstance().SaveUser(other, []string{"Passkeys"})

		opts, b := beginLogin(t)
		resp, _, _ := oa.Get(opts)
		if _, err := FinishLogin(httptest.NewRecorder(), b.request(), resp); !errors.Is(err, ErrUnknownCredential) {
			t.Errorf("err = %v, want ErrUnknownCredential", err)
		}
	})
}

func TestRegistrationIsBoundToUser(t *testing.T) {
	setup(t)
	alice := newUser(t, "pk-alice")
	bob := newUser(t, "pk-bob")
	a := virtualauth.New(testOrigin)

	b := &browser{}
	w := httptest.NewRecorder()
	creation, err := BeginRegistration(w, b.request(), alice)
	if err != nil {
		t.Fatal(err)
	}
	b.keep(w)
	opts, _ := json.Marshal(creation)
	resp, _, _ := a.Create(opts)
	if _, err := FinishRegistration(httptest.NewRecorder(), b.request(), bob, resp, ""); !errors.Is(err, ErrChallenge) {
		t.Errorf("err = %v, want ErrChallenge", err)
	}
}

func TestRegistrationNeedsUserVerification(t *testing.T) {
	setup(t)
	u := newUser(t, "pk-nouv")
	a := virtualauth.New(testOrigin)
	a.UserVerified = false

	b := &browser{}
	w := httptest.NewRecorder()
	creation, err := BeginRegistration(w, b.request(), u)
	if err != nil {
		t.Fatal(err)
	}
	b.keep(w)
	opts, _ := json.Marshal(creation)
	resp, _, _ := a.Create(opts)
	if _, err := FinishRegistration(httptest.NewRecorder(), b.request(), u, resp, ""); !errors.Is(err, ErrVerify) {
		t.Errorf("err = %v, want ErrVerify", err)
	}
	if len(u.Passkeys) != 0 {
		t.Errorf("passkey stored without user verification")
	}
}

func TestDisabledForIPAndLeaf(t *testing.T) {
	for _, cfg := range []*config.ServerConfig{
		{URL: "https://10.0.0.1:3000", EncryptionKey: "k"},
		{URL: testOrigin, EncryptionKey: "k", LeafNode: true},
		{URL: "", EncryptionKey: "k"},
	} {
		config.SetServerConfig(cfg)
		if Enabled() {
			t.Errorf("Enabled() with %+v", cfg)
		}
	}
	setup(t)
	if !Enabled() {
		t.Error("Enabled() false for a host name")
	}
}
