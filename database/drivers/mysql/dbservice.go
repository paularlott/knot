package driver_mysql

import (
	"fmt"

	"github.com/paularlott/knot/database/model"

	_ "github.com/go-sql-driver/mysql"
)

func (db *MySQLDriver) SaveDbService(service *model.DbService) error {
  tx, err := db.connection.Begin()
  if err != nil {
    return err
  }

  // Assume update
  result, err := tx.Exec("UPDATE dbservice SET name=?, db_type=?, db_host=?, db_port=?, db_user=?, db_password=?, proxy_host=?, proxy_port=?, proxy_user=?, proxy_password=? WHERE dbservice_id=?",
    service.Name, service.DbType, service.DbHost, service.DbPort, service.DbUser, service.DbPassword, service.ProxyHost, service.ProxyPort, service.ProxyUser, service.ProxyPassword, service.Id,
  )
  if err != nil {
    tx.Rollback()
    return err
  }

  // If no rows were updated then do an insert
  if rows, _ := result.RowsAffected(); rows == 0 {
    _, err = tx.Exec("INSERT INTO dbservice (dbservice_id, name, db_type, db_host, db_port, db_user, db_password, proxy_host, proxy_port, proxy_user, proxy_password) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
      service.Id, service.Name, service.DbType, service.DbHost, service.DbPort, service.DbUser, service.DbPassword, service.ProxyHost, service.ProxyPort, service.ProxyUser, service.ProxyPassword,
    )
    if err != nil {
      tx.Rollback()
      return err
    }
  }

  tx.Commit()

  return nil
}

func (db *MySQLDriver) DeleteDbService(service *model.DbService) error {
  _, err := db.connection.Exec("DELETE FROM dbservice WHERE dbservice_id = ?", service.Id)
  return err
}

func (db *MySQLDriver) getDbServices(where string, args ...interface{}) ([]*model.DbService, error) {
  var services []*model.DbService

  if where != "" {
    where = "WHERE " + where
  }

  rows, err := db.connection.Query(fmt.Sprintf("SELECT dbservice_id, name, db_host, db_port, db_user, db_password, proxy_host, proxy_port, proxy_user, proxy_password FROM dbservice %s ORDER BY name ASC", where), args ...)
  if err != nil {
    return nil, err
  }
  defer rows.Close()

  for rows.Next() {
    var service = &model.DbService{}

    err := rows.Scan(&service.Id, &service.Name, &service.DbHost, &service.DbPort, &service.DbUser, &service.DbPassword, &service.ProxyHost, &service.ProxyPort, &service.ProxyUser, &service.ProxyPassword)
    if err != nil {
      return nil, err
    }

    services = append(services, service)
  }

  return services, nil
}

func (db *MySQLDriver) GetDbService(id string) (*model.DbService, error) {
  services, err := db.getDbServices("dbservice_id=?", id)
  if err != nil {
    return nil, err
  }
  if len(services) == 0 {
    return nil, fmt.Errorf("service not found")
  }

  return services[0], nil
}

func (db *MySQLDriver) GetDbServiceByName(name string) (*model.DbService, error) {
  services, err := db.getDbServices("name=?",  name)
  if err != nil {
    return nil, err
  }
  if len(services) == 0 {
    return nil, fmt.Errorf("service not found")
  }

  return services[0], nil
}

func (db *MySQLDriver) GetDbServices() ([]*model.DbService, error) {
  services, err := db.getDbServices("")
  if err != nil {
    return nil, err
  }

  return services, nil
}
