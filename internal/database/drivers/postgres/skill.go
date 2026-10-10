package driver_postgres

import (
	"fmt"

	"github.com/paularlott/knot/internal/database/model"
)

func (db *PostgresDriver) SaveSkill(skill *model.Skill, updateFields []string) error {
	tx, err := db.connection.Begin()
	if err != nil {
		return err
	}

	var doUpdate bool
	err = tx.QueryRow("SELECT EXISTS(SELECT 1 FROM skills WHERE skill_id=?)", skill.Id).Scan(&doUpdate)
	if err != nil {
		tx.Rollback()
		return err
	}

	if doUpdate {
		err = db.update("skills", skill, updateFields)
	} else {
		err = db.create("skills", skill)
	}
	if err != nil {
		tx.Rollback()
		return err
	}

	tx.Commit()
	return nil
}

func (db *PostgresDriver) DeleteSkill(skill *model.Skill) error {
	_, err := db.connection.Exec("DELETE FROM skills WHERE skill_id = ?", skill.Id)
	return err
}

func (db *PostgresDriver) GetSkill(id string) (*model.Skill, error) {
	var skills []*model.Skill

	err := db.read("skills", &skills, nil, "skill_id = ?", id)
	if err != nil {
		return nil, err
	}
	if len(skills) == 0 {
		return nil, fmt.Errorf("skill not found")
	}

	return skills[0], nil
}

func (db *PostgresDriver) GetSkills() ([]*model.Skill, error) {
	var skills []*model.Skill

	err := db.read("skills", &skills, nil, "TRUE ORDER BY name")
	return skills, err
}

func (db *PostgresDriver) GetSkillsByName(name string) ([]*model.Skill, error) {
	var skills []*model.Skill

	err := db.read("skills", &skills, nil, "name = ? ORDER BY "+zonesLength+" DESC, created_at", name)
	if err != nil {
		return nil, err
	}
	if len(skills) == 0 {
		return nil, fmt.Errorf("skill not found")
	}

	return skills, nil
}

func (db *PostgresDriver) GetSkillsByNameAndUser(name string, userId string) ([]*model.Skill, error) {
	var skills []*model.Skill

	err := db.read("skills", &skills, nil, "name = ? AND user_id = ? ORDER BY "+zonesLength+" DESC, created_at", name, userId)
	if err != nil {
		return nil, err
	}
	if len(skills) == 0 {
		return nil, fmt.Errorf("skill not found")
	}

	return skills, nil
}
