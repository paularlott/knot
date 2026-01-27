import Alpine from "alpinejs";

window.skillListComponent = function (userId, zone, permissionManageSkills, isLeafNode) {
  document.addEventListener("keydown", (e) => {
    if ((e.metaKey || e.ctrlKey) && e.key === "k") {
      e.preventDefault();
      document.getElementById("search").focus();
    }
  });

  const defaultShowMySkills = true;
  const defaultShowGlobalSkills = false;

  return {
    loading: true,
    deleteConfirm: {
      show: false,
      skill: {
        skill_id: "",
        name: "",
      },
    },
    skillFormModal: {
      show: false,
      isEdit: false,
      skillId: "",
      isUserSkill: false,
    },
    skills: [],
    availableZones: [],
    showMySkills: Alpine.$persist(defaultShowMySkills)
      .as("skill-show-my-skills")
      .using(sessionStorage),
    showGlobalSkills: Alpine.$persist(defaultShowGlobalSkills)
      .as("skill-show-global-skills")
      .using(sessionStorage),
    showLocalSkills: Alpine.$persist(true)
      .as("skill-show-local-skills")
      .using(sessionStorage),
    showAllZones: Alpine.$persist(false)
      .as("skill-show-all-zones")
      .using(sessionStorage),
    searchTerm: Alpine.$persist("")
      .as("skill-search-term")
      .using(sessionStorage),
    currentUserId: userId || "",
    currentZone: zone || "",
    permissionManageSkills: permissionManageSkills || false,
    isLeafNode: isLeafNode || false,

    async init() {
      await this.getSkills();

      if (window.sseClient) {
        window.sseClient.subscribe("skills:changed", (payload) => {
          if (payload?.id) this.getSkills(payload.id);
        });

        window.sseClient.subscribe("skills:deleted", (payload) => {
          this.skills = this.skills.filter(
            (x) => x.skill_id !== payload?.id,
          );
          this.applyFilters();
        });

        window.sseClient.subscribe("reconnected", () => {
          this.getSkills();
        });
      }
    },

    async getSkills(skillId) {
      const url = skillId
        ? `/api/skill/${skillId}`
        : `/api/skill?all_zones=${this.showAllZones}`;
      const groupsResponse = await fetch("/api/groups");
      const groupsData =
        groupsResponse.status === 200
          ? await groupsResponse.json()
          : { groups: [] };
      const groupsMap = {};
      groupsData.groups.forEach((g) => (groupsMap[g.group_id] = g.name));

      // Fetch global skills
      await fetch(url, {
        headers: {
          "Content-Type": "application/json",
        },
      })
        .then((response) => {
          if (response.status === 200) {
            response.json().then((data) => {
              const skillList = skillId ? [data] : data.skills;
              skillList.forEach((skill) => {
                skill.group_names = (skill.groups || []).map(
                  (gid) => groupsMap[gid] || gid,
                );
                const index = this.skills.findIndex(
                  (s) => s.skill_id === skill.skill_id,
                );
                if (index >= 0) {
                  this.skills[index] = skill;
                } else {
                  this.skills.push(skill);
                }

                // Collect zones
                if (skill.zones && skill.zones.length) {
                  skill.zones.forEach((z) => {
                    if (!this.availableZones.includes(z)) {
                      this.availableZones.push(z);
                    }
                  });
                }
              });

              this.skills.sort((a, b) => a.name.localeCompare(b.name));
              this.availableZones.sort();
              this.applyFilters();
              this.loading = false;
            });
          } else if (response.status === 401) {
            window.location.href = "/logout";
          }
        })
        .catch(() => {});

      // Fetch user skills if user has permission
      if (this.currentUserId && !skillId) {
        await fetch(
          `/api/skill?user_id=${this.currentUserId}&all_zones=${this.showAllZones}`,
          {
            headers: {
              "Content-Type": "application/json",
            },
          },
        )
          .then((response) => {
            if (response.status === 200) {
              response.json().then((data) => {
                data.skills.forEach((skill) => {
                  skill.group_names = [];
                  const index = this.skills.findIndex(
                    (s) => s.skill_id === skill.skill_id,
                  );
                  if (index >= 0) {
                    this.skills[index] = skill;
                  } else {
                    this.skills.push(skill);
                  }

                  if (skill.zones && skill.zones.length) {
                    skill.zones.forEach((z) => {
                      if (!this.availableZones.includes(z)) {
                        this.availableZones.push(z);
                      }
                    });
                  }
                });

                this.skills.sort((a, b) => a.name.localeCompare(b.name));
                this.availableZones.sort();
                this.applyFilters();
              });
            } else if (response.status === 401) {
              window.location.href = "/logout";
            }
            this.loading = false;
          })
          .catch(() => {
            this.loading = false;
          });
      } else {
        this.loading = false;
      }
    },

    createSkill(isUserSkill = false) {
      this.skillFormModal.isEdit = false;
      this.skillFormModal.skillId = "";
      this.skillFormModal.isUserSkill = isUserSkill;
      this.skillFormModal.show = true;

      if (isUserSkill) {
        this.showMySkills = true;
      } else {
        this.showGlobalSkills = true;
      }
    },

    editSkill(skillId) {
      const skill = this.skills.find((s) => s.skill_id === skillId);
      this.skillFormModal.isEdit = true;
      this.skillFormModal.skillId = skillId;
      this.skillFormModal.isUserSkill =
        skill && skill.user_id ? true : false;
      this.skillFormModal.show = true;
    },

    canEditSkill(skill) {
      if (this.isLeafNode) return true;
      if (skill.user_id && skill.user_id === this.currentUserId) return true;
      if (!skill.user_id) return this.permissionManageSkills;
      return false;
    },

    canActuallyEditSkill(skill) {
      if (skill.is_managed) return false;
      if (skill.user_id && skill.user_id === this.currentUserId) return true;
      if (!skill.user_id) return this.permissionManageSkills;
      return false;
    },

    canDeleteSkill(skill) {
      if (this.isLeafNode) return false;
      if (skill.user_id && skill.user_id === this.currentUserId) return true;
      if (!skill.user_id) return this.permissionManageSkills;
      return false;
    },

    async deleteSkill(skillId) {
      await fetch(`/api/skill/${skillId}`, {
        method: "DELETE",
        headers: {
          "Content-Type": "application/json",
        },
      })
        .then((response) => {
          if (response.status === 200) {
            this.$dispatch("show-alert", {
              msg: "Skill deleted",
              type: "success",
            });
          } else if (response.status === 401) {
            window.location.href = "/logout";
          } else {
            this.$dispatch("show-alert", {
              msg: "Skill could not be deleted",
              type: "error",
            });
          }
        })
        .catch(() => {});
      this.getSkills();
    },

    filterChanged() {
      this.$nextTick(() => {
        this.applyFilters();
      });
    },

    showAllZonesChanged() {
      this.getSkills();
    },

    searchChanged() {
      this.applyFilters();
    },

    applyFilters() {
      const term = this.searchTerm.toLowerCase();
      this.skills.forEach((s) => {
        let showRow = true;

        const isGlobal = !s.user_id;
        const isMine = s.user_id === this.currentUserId;
        const matchesFilter = (isGlobal && this.showGlobalSkills) || (isMine && this.showMySkills);
        if (!matchesFilter) showRow = false;

        if (this.isLeafNode && this.showLocalSkills && s.is_managed) showRow = false;

        if (!this.showAllZones && this.currentZone) {
          const zones = s.zones || [];
          if (zones.length > 0) {
            if (!zones.includes(this.currentZone)) showRow = false;
          }
        }

        if (term.length > 0) {
          const inName = s.name.toLowerCase().includes(term);
          const inDesc = s.description.toLowerCase().includes(term);
          showRow = showRow && (inName || inDesc);
        }

        s.searchHide = !showRow;
      });
    },
  };
};
