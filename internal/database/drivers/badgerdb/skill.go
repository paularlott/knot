package driver_badgerdb

import (
	"encoding/json"
	"fmt"
	"sort"

	badger "github.com/dgraph-io/badger/v4"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/util"
)

func (db *BadgerDbDriver) SaveSkill(skill *model.Skill, updateFields []string) error {
	err := db.connection.Update(func(txn *badger.Txn) error {
		existingSkill, _ := db.GetSkill(skill.Id)

		if existingSkill != nil {
			if (existingSkill.Name != skill.Name || existingSkill.UserId != skill.UserId) && (len(updateFields) == 0 || util.InArray(updateFields, "Name") || util.InArray(updateFields, "UserId")) {
				err := txn.Delete([]byte(fmt.Sprintf("SkillsByName:%s:%s", existingSkill.UserId, existingSkill.Name)))
				if err != nil {
					return err
				}
			}

			if len(updateFields) > 0 {
				util.CopyFields(skill, existingSkill, updateFields)
				skill = existingSkill
			}
		}

		data, err := json.Marshal(skill)
		if err != nil {
			return err
		}

		e := badger.NewEntry([]byte(fmt.Sprintf("Skills:%s", skill.Id)), data)
		if err = txn.SetEntry(e); err != nil {
			return err
		}

		e = badger.NewEntry([]byte(fmt.Sprintf("SkillsByName:%s:%s", skill.UserId, skill.Name)), []byte(skill.Id))
		if err = txn.SetEntry(e); err != nil {
			return err
		}

		return nil
	})

	return err
}

func (db *BadgerDbDriver) DeleteSkill(skill *model.Skill) error {
	err := db.connection.Update(func(txn *badger.Txn) error {
		err := txn.Delete([]byte(fmt.Sprintf("Skills:%s", skill.Id)))
		if err != nil {
			return err
		}

		err = txn.Delete([]byte(fmt.Sprintf("SkillsByName:%s:%s", skill.UserId, skill.Name)))
		if err != nil {
			return err
		}

		return nil
	})

	return err
}

func (db *BadgerDbDriver) GetSkill(id string) (*model.Skill, error) {
	var skill = &model.Skill{}

	err := db.connection.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(fmt.Sprintf("Skills:%s", id)))
		if err != nil {
			return err
		}

		return item.Value(func(val []byte) error {
			return json.Unmarshal(val, skill)
		})
	})

	if err != nil {
		return nil, err
	}

	return skill, err
}

func (db *BadgerDbDriver) GetSkills() ([]*model.Skill, error) {
	var skills []*model.Skill

	err := db.connection.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		prefix := []byte("Skills:")

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			var skill = &model.Skill{}

			err := item.Value(func(val []byte) error {
				return json.Unmarshal(val, skill)
			})
			if err != nil {
				return err
			}

			skills = append(skills, skill)
		}

		return nil
	})

	sort.Slice(skills, func(i, j int) bool {
		return skills[i].Name < skills[j].Name
	})

	return skills, err
}

func (db *BadgerDbDriver) GetSkillsByName(name string) ([]*model.Skill, error) {
	var skills []*model.Skill

	err := db.connection.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		prefix := []byte("Skills:")

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			var skill = &model.Skill{}

			err := item.Value(func(val []byte) error {
				return json.Unmarshal(val, skill)
			})
			if err != nil {
				return err
			}

			if skill.Name == name && skill.UserId == "" {
				skills = append(skills, skill)
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	if len(skills) == 0 {
		return nil, fmt.Errorf("skill not found")
	}

	sort.Slice(skills, func(i, j int) bool {
		zonesI := len(skills[i].Zones)
		zonesJ := len(skills[j].Zones)
		if zonesI != zonesJ {
			return zonesI > zonesJ
		}
		return skills[i].CreatedAt.Before(skills[j].CreatedAt)
	})

	return skills, nil
}

func (db *BadgerDbDriver) GetSkillsByNameAndUser(name string, userId string) ([]*model.Skill, error) {
	var skills []*model.Skill

	err := db.connection.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		prefix := []byte("Skills:")

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			var skill = &model.Skill{}

			err := item.Value(func(val []byte) error {
				return json.Unmarshal(val, skill)
			})
			if err != nil {
				return err
			}

			if skill.Name == name && skill.UserId == userId {
				skills = append(skills, skill)
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	if len(skills) == 0 {
		return nil, fmt.Errorf("skill not found")
	}

	sort.Slice(skills, func(i, j int) bool {
		zonesI := len(skills[i].Zones)
		zonesJ := len(skills[j].Zones)
		if zonesI != zonesJ {
			return zonesI > zonesJ
		}
		return skills[i].CreatedAt.Before(skills[j].CreatedAt)
	})

	return skills, nil
}
