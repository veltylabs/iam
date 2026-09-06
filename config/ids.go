package config

import (
	"webtyp.com/model"
	"webtyp.com/unixid"
)

// NewIDs construye el generador de IDs del servicio. Es el único sitio que
// instancia unixid: cambiar de implementación (o su configuración) es un
// cambio en un archivo, no una cacería por el repo.
func NewIDs() (model.IDGenerator, error) {
	return unixid.NewUnixID()
}
