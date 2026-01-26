package driver_mysql

import (
	"fmt"

	"github.com/paularlott/knot/internal/database/model"

	_ "github.com/go-sql-driver/mysql"
)

func (db *MySQLDriver) SaveSkill(skill *model.Skill, updateFields []string) error {
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

func (db *MySQLDriver) DeleteSkill(skill *model.Skill) error {
	_, err := db.connection.Exec("DELETE FROM skills WHERE skill_id = ?", skill.Id)
	return err
}

func (db *MySQLDriver) GetSkill(id string) (*model.Skill, error) {
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

func (db *MySQLDriver) GetSkills() ([]*model.Skill, error) {
	var skills []*model.Skill

	err := db.read("skills", &skills, nil, "1 ORDER BY name")
	return skills, err
}

func (db *MySQLDriver) GetSkillsByName(name string) ([]*model.Skill, error) {
	var skills []*model.Skill

	err := db.read("skills", &skills, nil, "name = ? ORDER BY JSON_LENGTH(zones) DESC, created_at", name)
	if err != nil {
		return nil, err
	}
	if len(skills) == 0 {
		return nil, fmt.Errorf("skill not found")
	}

	return skills, nil
}

func (db *MySQLDriver) GetSkillsByNameAndUser(name string, userId string) ([]*model.Skill, error) {
	var skills []*model.Skill

	err := db.read("skills", &skills, nil, "name = ? AND user_id = ? ORDER BY JSON_LENGTH(zones) DESC, created_at", name, userId)
	if err != nil {
		return nil, err
	}
	if len(skills) == 0 {
		return nil, fmt.Errorf("skill not found")
	}

	return skills, nil
}
