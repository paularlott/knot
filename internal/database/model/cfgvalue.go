package model

// CfgValue object
type CfgValue struct {
	Name  string `json:"name" db:"name,pk" msgpack:"name"`
	Value string `json:"value" db:"value" msgpack:"value"`
}
