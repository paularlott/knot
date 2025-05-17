package model

// CfgValue object
type CfgValue struct {
	Name  string `json:"name" db:"name" msgpack:"name"`
	Value string `json:"value" db:"value" msgpack:"value"`
}

func NewCfgValue(name string, value string) *CfgValue {
	return &CfgValue{
		Name:  name,
		Value: value,
	}
}
