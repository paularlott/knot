import Alpine from "alpinejs";
import { focus } from "../focus.js";

window.apiTokensComponent = function () {
  document.addEventListener("keydown", (e) => {
    if ((e.metaKey || e.ctrlKey) && e.key === "k") {
      e.preventDefault();
      document.getElementById("search").focus();
    }
  });

  // Available scopes. Currently just "methods". Each entry has:
  //   value: the scope string stored in the DB
  //   label: human-readable label for the checkbox
  //   description: short help text
  const availableScopes = [
    {
      value: "methods",
      label: "Methods",
      description: "Discover and call space methods",
    },
    {
      value: "mcp",
      label: "MCP",
      description: "Use the MCP server endpoint (/mcp)",
    },
  ];

  // Helper: build a scope-checkbox state object from the available scopes,
  // defaulting to all checked (scoped mode assumes you want every scope
  // unless you explicitly untick one). Called for form initialization.
  function defaultScopeState() {
    const state = {};
    availableScopes.forEach((s) => {
      state[s.value] = true;
    });
    return state;
  }

  // Helper: build an empty scope state (all unchecked).
  function emptyScopeState() {
    const state = {};
    availableScopes.forEach((s) => {
      state[s.value] = false;
    });
    return state;
  }

  return {
    loading: true,
    tokens: [],
    availableScopes,

    // Create modal state
    tokenFormModal: { show: false },
    createForm: {
      name: "",
      fullAccess: true,
      scopes: defaultScopeState(),
    },
    nameValid: true,

    // Edit modal state
    editModal: { show: false },
    editForm: {
      tokenId: "",
      name: "",
      fullAccess: false,
      scopes: defaultScopeState(),
    },
    editNameValid: true,

    deleteConfirm: {
      show: false,
      token: { token_id: "", name: "" },
    },
    searchTerm: Alpine.$persist("")
      .as("apitoken-search-term")
      .using(sessionStorage),

    async init() {
      await this.getTokens();

      this.$watch("tokenFormModal.show", (value) => {
        if (value) {
          focus.Element('input[name="name"]');
        } else {
          this.createForm.name = "";
          this.createForm.fullAccess = true;
          this.createForm.scopes = defaultScopeState();
          this.nameValid = true;
        }
      });

      this.$watch("editModal.show", (value) => {
        if (!value) {
          this.editForm.tokenId = "";
          this.editForm.name = "";
          this.editForm.fullAccess = false;
          this.editForm.scopes = defaultScopeState();
          this.editNameValid = true;
        }
      });

      window.addEventListener("close-token-form", () => {
        this.tokenFormModal.show = false;
        this.getTokens();
      });

      if (window.sseClient) {
        window.sseClient.subscribe("tokens:changed", () => {
          this.getTokens();
        });
        window.sseClient.subscribe("tokens:deleted", (payload) => {
          this.tokens = this.tokens.filter((t) => t.token_id !== payload?.id);
          this.searchChanged();
        });
        window.sseClient.subscribe("reconnected", () => {
          this.getTokens();
        });
      }
    },

    async getTokens() {
      await fetch("/api/tokens", {
        headers: { "Content-Type": "application/json" },
      })
        .then((response) => {
          if (response.status === 200) {
            response.json().then((tokens) => {
              this.tokens = tokens;
              this.searchChanged();
              this.loading = false;
            });
          } else if (response.status === 401) {
            window.location.href = "/logout";
          }
        })
        .catch(() => {});
    },

    // ---- Helpers for scope UI ----

    // Build the list of selected scope strings from a form's scope-checkbox map.
    selectedScopes(scopeMap) {
      return this.availableScopes
        .filter((s) => scopeMap[s.value])
        .map((s) => s.value);
    },

    // True if at least one scope is selected (used for form validation when
    // not in full-access mode).
    hasAnyScope(scopeMap) {
      return this.selectedScopes(scopeMap).length > 0;
    },

    // Human-readable summary of a token's scopes for the list table.
    scopeLabel(token) {
      if (!token.scopes || token.scopes.length === 0) {
        return "Full Access";
      }
      return token.scopes
        .map((s) => {
          const match = this.availableScopes.find((a) => a.value === s);
          return match ? match.label : s;
        })
        .join(", ");
    },

    // Returns an array of human-readable scope labels for rendering as individual pills.
    scopeLabelList(token) {
      if (!token.scopes || token.scopes.length === 0) {
        return [];
      }
      return token.scopes.map((s) => {
        const match = this.availableScopes.find((a) => a.value === s);
        return match ? match.label : s;
      });
    },

    scopeBadgeClass(token) {
      if (!token.scopes || token.scopes.length === 0) return "app-badge-neutral";
      return "app-badge-info";
    },

    // ---- Create ----

    createToken() {
      this.tokenFormModal.show = true;
    },

    checkName() {
      this.nameValid =
        this.createForm.name.length > 0 && this.createForm.name.length < 255;
      return this.nameValid;
    },

    async submitTokenForm() {
      let err = !this.checkName();
      if (
        !this.createForm.fullAccess &&
        !this.hasAnyScope(this.createForm.scopes)
      ) {
        this.$dispatch("show-alert", {
          msg: "Select at least one scope or enable Full Access.",
          type: "error",
        });
        return;
      }
      if (err) return;

      this.loading = true;

      const data = {
        name: this.createForm.name,
        scopes: this.createForm.fullAccess
          ? null
          : this.selectedScopes(this.createForm.scopes),
      };

      await fetch("/api/tokens", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(data),
      })
        .then((response) => {
          if (response.status === 201) {
            this.$dispatch("show-alert", {
              msg: "Token created",
              type: "success",
            });
            this.tokenFormModal.show = false;
            this.getTokens();
          } else {
            this.$dispatch("show-alert", {
              msg: "Failed to create API token",
              type: "error",
            });
          }
        })
        .catch((error) => {
          this.$dispatch("show-alert", {
            msg: `Error!<br />${error.message}`,
            type: "error",
          });
        })
        .finally(() => {
          this.loading = false;
        });
    },

    // ---- Edit ----

    openEdit(token) {
      this.editForm.tokenId = token.token_id;
      this.editForm.name = token.name;
      this.editNameValid = true;
      if (!token.scopes || token.scopes.length === 0) {
        this.editForm.fullAccess = true;
        this.editForm.scopes = emptyScopeState();
      } else {
        this.editForm.fullAccess = false;
        this.editForm.scopes = emptyScopeState();
        token.scopes.forEach((s) => {
          this.editForm.scopes[s] = true;
        });
      }
      this.editModal.show = true;
    },

    checkEditName() {
      this.editNameValid =
        this.editForm.name.length > 0 && this.editForm.name.length < 255;
      return this.editNameValid;
    },

    async submitEditForm() {
      if (!this.checkEditName()) return;
      if (
        !this.editForm.fullAccess &&
        !this.hasAnyScope(this.editForm.scopes)
      ) {
        this.$dispatch("show-alert", {
          msg: "Select at least one scope or enable Full Access.",
          type: "error",
        });
        return;
      }

      this.loading = true;

      const scopes = this.editForm.fullAccess
        ? []
        : this.selectedScopes(this.editForm.scopes);
      const data = {
        name: this.editForm.name,
        scopes: scopes,
      };

      await fetch(`/api/tokens/${this.editForm.tokenId}`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(data),
      })
        .then((response) => {
          if (response.status === 200) {
            this.$dispatch("show-alert", {
              msg: "Token updated",
              type: "success",
            });
            this.editModal.show = false;
            this.getTokens();
          } else {
            this.$dispatch("show-alert", {
              msg: "Failed to update token",
              type: "error",
            });
          }
        })
        .catch((error) => {
          this.$dispatch("show-alert", {
            msg: `Error!<br />${error.message}`,
            type: "error",
          });
        })
        .finally(() => {
          this.loading = false;
        });
    },

    // ---- Delete ----

    async deleteToken(tokenId) {
      const self = this;
      await fetch(`/api/tokens/${tokenId}`, {
        method: "DELETE",
        headers: { "Content-Type": "application/json" },
      })
        .then((response) => {
          if (response.status === 200) {
            self.$dispatch("show-alert", {
              msg: "Token deleted",
              type: "success",
            });
          } else if (response.status === 401) {
            window.location.href = "/logout";
          } else {
            self.$dispatch("show-alert", {
              msg: "Token could not be deleted",
              type: "error",
            });
          }
        })
        .catch(() => {});
      this.getTokens();
    },

    // ---- Table helpers ----

    searchChanged() {
      const term = this.searchTerm.toLowerCase();
      this.tokens.forEach((t) => {
        if (term.length === 0) {
          t.searchHide = false;
        } else {
          t.searchHide = !t.name.toLowerCase().includes(term);
        }
      });
    },

    async copyToClipboard(text) {
      await navigator.clipboard.writeText(text);
      this.$dispatch("show-alert", {
        msg: "Copied to clipboard",
        type: "success",
      });
    },
  };
};
