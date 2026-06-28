package driver_mysql

import (
	"fmt"

	"github.com/paularlott/knot/internal/database/model"

	_ "github.com/go-sql-driver/mysql"
)

func (db *MySQLDriver) SaveEventSink(sink *model.EventSink, updateFields []string) error {
	tx, err := db.connection.Begin()
	if err != nil {
		return err
	}

	var doUpdate bool
	err = tx.QueryRow("SELECT EXISTS(SELECT 1 FROM event_sinks WHERE event_sink_id=?)", sink.Id).Scan(&doUpdate)
	if err != nil {
		tx.Rollback()
		return err
	}

	if doUpdate {
		err = db.update("event_sinks", sink, updateFields)
	} else {
		err = db.create("event_sinks", sink)
	}
	if err != nil {
		tx.Rollback()
		return err
	}

	tx.Commit()
	return nil
}

func (db *MySQLDriver) DeleteEventSink(sink *model.EventSink) error {
	_, err := db.connection.Exec("DELETE FROM event_sinks WHERE event_sink_id = ?", sink.Id)
	return err
}

func (db *MySQLDriver) GetEventSink(id string) (*model.EventSink, error) {
	var sinks []*model.EventSink

	err := db.read("event_sinks", &sinks, nil, "event_sink_id = ?", id)
	if err != nil {
		return nil, err
	}
	if len(sinks) == 0 {
		return nil, fmt.Errorf("event sink not found")
	}

	return sinks[0], nil
}

func (db *MySQLDriver) GetEventSinks() ([]*model.EventSink, error) {
	var sinks []*model.EventSink

	err := db.read("event_sinks", &sinks, nil, "1 ORDER BY name")
	return sinks, err
}
