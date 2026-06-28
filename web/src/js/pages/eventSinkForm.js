/// webhook body template editor
import ace from "ace-builds/src-noconflict/ace";
import "ace-builds/src-noconflict/mode-text";
import "ace-builds/src-noconflict/theme-github";
import "ace-builds/src-noconflict/theme-github_dark";
import "ace-builds/src-noconflict/ext-searchbox";
import "ace-builds/src-noconflict/ext-language_tools";

import { focus } from "../focus.js";
import { setSpecCompleter } from "./aceSpecCompleter.js";
import { eventBodyCompletions } from "./eventCompletions.js";

window.eventSinkForm = function (isEdit, sinkId, isGlobal = false) {
  return {
    loading: true,
    isEdit: isEdit,
    sinkId: sinkId,
    isGlobal: isGlobal,
    nameValid: true,
    eventsStr: "",
    headerPairs: [],
    scriptList: [],
    scriptSearch: "",
    scriptShowList: false,
    scriptSelectedIndex: -1,
    showSecret: false,
    bodyEditor: null,
    formData: {
      name: "",
      description: "",
      events: [],
      sink_type: "webhook",
      webhook: {
        url: "",
        secret: "",
        headers: {},
        body_template: "",
        skip_tls_verify: false,
      },
      script_id: "",
      active: true,
    },
    darkMode: Alpine.$persist(null).as("dark-theme").using(localStorage),

    init() {
      this.$watch("scriptSearch", (value) => {
        if (value === "" && this.formData.sink_type === "script") {
          this.formData.script_id = "";
        }
      });
    },

    async initData() {
      focus.Element('input[name="name"]');

      await this.loadScripts();

      if (this.isEdit) {
        await fetch(`/api/event-sinks/${this.sinkId}`, {
          headers: {
            "Content-Type": "application/json",
          },
        })
          .then((response) => {
            if (response.status === 200) {
              response.json().then((data) => {
                this.formData = {
                  name: data.name,
                  description: data.description,
                  events: data.events || [],
                  sink_type: data.sink_type || "webhook",
                  webhook: {
                    url: data.webhook ? data.webhook.url : "",
                    secret: data.webhook ? data.webhook.secret : "",
                    headers: data.webhook && data.webhook.headers ? data.webhook.headers : {},
                    body_template: data.webhook ? data.webhook.body_template : "",
                    skip_tls_verify: data.webhook ? data.webhook.skip_tls_verify : false,
                  },
                  script_id: data.script_id || "",
                  active: data.active,
                };
                this.isGlobal = !data.user_id;
                this.eventsStr = this.formData.events.join(", ");
                // Convert headers map to editable pairs
                this.headerPairs = Object.keys(this.formData.webhook.headers).map(
                  (k) => ({ key: k, value: this.formData.webhook.headers[k] }),
                );
                this.initEditors();
                this.syncScriptSearch();
                this.loading = false;
              });
            } else if (response.status === 401) {
              window.location.href = "/logout";
            }
          })
          .catch(() => {});
      } else {
        this.formData.webhook.secret = this.generateSecret();
        this.formData.webhook.body_template = this.defaultBodyTemplate();
        this.initEditors();
        this.loading = false;
      }
    },

    defaultBodyTemplate() {
      return [
        "{",
        '  "event_id":   "${{ .event.id }}",',
        '  "event_type": "${{ .event.type }}",',
        '  "event_ts":   "${{ .event.ts }}",',
        '  "data": ${{ json .event.data }}',
        "}",
      ].join("\n");
    },

    syncScriptSearch() {
      if (this.formData.script_id) {
        const script = this.scriptList.find(
          (s) => s.script_id === this.formData.script_id,
        );
        this.scriptSearch = script ? script.name : "";
      } else {
        this.scriptSearch = "";
      }
    },

    generateSecret() {
      const bytes = new Uint8Array(32);
      crypto.getRandomValues(bytes);
      return Array.from(bytes)
        .map((b) => b.toString(16).padStart(2, "0"))
        .join("");
    },

    initEditors() {
      this.$nextTick(() => {
        let darkMode = JSON.parse(localStorage.getItem("_x_darkMode"));
        if (darkMode == null) darkMode = true;

        const initBodyEditor = () => {
          const editorId = this.formData.sink_type === "json-rpc" ? "jsonrpc_body_template" : "body_template";
          if (!this.bodyEditor) {
            const el = document.getElementById(editorId);
            if (!el) return;
            this.bodyEditor = ace.edit(editorId);
            this.bodyEditor.session.setValue(this.formData.webhook.body_template);
            this.bodyEditor.session.on("change", () => {
              this.formData.webhook.body_template = this.bodyEditor.getValue();
            });
            this.bodyEditor.setTheme(
              darkMode ? "ace/theme/github_dark" : "ace/theme/github",
            );
            this.bodyEditor.session.setMode("ace/mode/text");

            setSpecCompleter(this.bodyEditor, eventBodyCompletions);

            this.bodyEditor.setOptions({
              printMargin: false,
              newLineMode: "unix",
              tabSize: 2,
              wrap: false,
              vScrollBarAlwaysVisible: true,
              customScrollbar: true,
              useWorker: false,
              enableBasicAutocompletion: true,
              enableLiveAutocompletion: true,
              enableSnippets: false,
            });
          } else {
            this.bodyEditor.session.setValue(this.formData.webhook.body_template);
          }
        };

        // The webhook section may be hidden via x-show; ensure the editor
        // container is visible before measuring.
        if (this.formData.sink_type === "webhook" || this.formData.sink_type === "json-rpc") {
          initBodyEditor();
        }

        window.addEventListener("theme-change", (e) => {
          if (this.bodyEditor) {
            if (e.detail.dark_theme) {
              this.bodyEditor.setTheme("ace/theme/github_dark");
            } else {
              this.bodyEditor.setTheme("ace/theme/github");
            }
          }
        });
      });
    },

    // Initialize the body editor lazily when switching to the webhook type
    ensureBodyEditor() {
      if (this.formData.sink_type === "webhook" || this.formData.sink_type === "json-rpc") {
        this.$nextTick(() => {
          const expectedId = this.formData.sink_type === "json-rpc" ? "jsonrpc_body_template" : "body_template";
          if (this.bodyEditor && this.bodyEditor.container && this.bodyEditor.container.id !== expectedId) {
            this.bodyEditor.destroy();
            this.bodyEditor = null;
          }
          if (!this.bodyEditor) {
            this.initEditors();
          }
          if (this.bodyEditor) {
            this.bodyEditor.resize();
          }
        });
      }
    },

    async loadScripts() {
      try {
        const response = await fetch("/api/scripts", {
          headers: { "Content-Type": "application/json" },
        });
        if (response.status === 200) {
          const data = await response.json();
          this.scriptList = (data.scripts || []).filter(
            (s) => s.script_type === "script" && s.active,
          );
        }
      } catch (e) {
        // ignore
      }
    },

    get filteredScripts() {
      if (!this.scriptList || this.scriptList.length === 0) return [];
      const q = (this.scriptSearch || "").toLowerCase();
      if (!q) return this.scriptList.slice(0, 50);
      return this.scriptList
        .filter((s) => s.name.toLowerCase().includes(q))
        .slice(0, 50);
    },

    scriptKeydown(e) {
      if (!this.scriptShowList) return;
      const opts = this.filteredScripts;
      if (e.key === "ArrowDown") {
        e.preventDefault();
        this.scriptSelectedIndex =
          this.scriptSelectedIndex < opts.length - 1
            ? this.scriptSelectedIndex + 1
            : 0;
      } else if (e.key === "ArrowUp") {
        e.preventDefault();
        this.scriptSelectedIndex =
          this.scriptSelectedIndex > 0
            ? this.scriptSelectedIndex - 1
            : opts.length - 1;
      } else if (e.key === "Enter" && this.scriptSelectedIndex >= 0) {
        e.preventDefault();
        this.scriptSelect(opts[this.scriptSelectedIndex]);
      } else if (e.key === "Escape") {
        this.scriptShowList = false;
        this.scriptSelectedIndex = -1;
      }
    },

    scriptSelect(option) {
      this.scriptSearch = option.name;
      this.formData.script_id = option.script_id;
      this.scriptShowList = false;
      this.scriptSelectedIndex = -1;
    },

    checkName() {
      this.nameValid = /^[a-zA-Z0-9_]{1,64}$/.test(this.formData.name);
    },

    addHeader() {
      this.headerPairs.push({ key: "", value: "" });
    },

    removeHeader(index) {
      this.headerPairs.splice(index, 1);
    },

    async submitData(continueEditing = false) {
      this.checkName();

      if (!this.nameValid) {
        return;
      }

      // Parse events from comma-separated string
      this.formData.events = this.eventsStr
        .split(",")
        .map((e) => e.trim())
        .filter((e) => e.length > 0);

      // Build headers map from pairs
      const headers = {};
      this.headerPairs.forEach((pair) => {
        const key = pair.key.trim();
        if (key.length > 0) {
          headers[key] = pair.value;
        }
      });

      const submitData = {
        name: this.formData.name,
        description: this.formData.description,
        events: this.formData.events,
        sink_type: this.formData.sink_type,
        script_id: this.formData.sink_type === "script" ? this.formData.script_id : "",
        active: this.formData.active,
      };

      if (this.formData.sink_type === "webhook") {
        submitData.webhook = {
          url: this.formData.webhook.url,
          secret: this.formData.webhook.secret,
          headers: headers,
          body_template: this.formData.webhook.body_template,
          skip_tls_verify: this.formData.webhook.skip_tls_verify,
        };
      } else if (this.formData.sink_type === "json-rpc") {
        submitData.webhook = {
          body_template: this.formData.webhook.body_template,
        };
      }

      // Set owner: "current" for own sinks, "" for global sinks
      if (!this.isGlobal) {
        submitData.user_id = "current";
      } else {
        submitData.user_id = "";
      }

      if (!continueEditing) {
        this.loading = true;
      }

      const url = this.isEdit
        ? `/api/event-sinks/${this.sinkId}`
        : "/api/event-sinks";
      const method = this.isEdit ? "PUT" : "POST";

      await fetch(url, {
        method: method,
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify(submitData),
      })
        .then(async (response) => {
          if (response.status === 200) {
            this.$dispatch("show-alert", {
              msg: "Event sink updated",
              type: "success",
            });
            if (!continueEditing) {
              this.$dispatch("close-event-sink-form");
            }
          } else if (response.status === 201) {
            const data = await response.json();
            this.$dispatch("show-alert", {
              msg: "Event sink created",
              type: "success",
            });
            if (continueEditing) {
              this.sinkId = data.event_sink_id;
              this.isEdit = true;
            } else {
              this.$dispatch("close-event-sink-form");
            }
          } else if (response.status === 401) {
            window.location.href = "/logout";
          } else {
            try {
              const data = await response.json();
              this.$dispatch("show-alert", {
                msg: data.error || "Failed to save event sink",
                type: "error",
              });
            } catch (e) {
              const text = await response.text();
              this.$dispatch("show-alert", {
                msg: text || "Failed to save event sink",
                type: "error",
              });
            }
          }
          this.loading = false;
        })
        .catch((err) => {
          this.$dispatch("show-alert", {
            msg: "Network error: " + err.message,
            type: "error",
          });
          this.loading = false;
        });
    },
  };
};
