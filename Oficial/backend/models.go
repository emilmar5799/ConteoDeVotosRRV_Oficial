package main

import (
	"time"
)

type DistribucionTerritorial struct {
	CodigoTerritorial int    `gorm:"primaryKey;autoIncrement:false"`
	Departamento      string `gorm:"size:100"`
	Provincia         string `gorm:"size:100"`
	Municipio         string `gorm:"size:100"`
}

func (DistribucionTerritorial) TableName() string {
	return "distribucion_territorial"
}

type RecintoElectoral struct {
	CodigoRecinto     int64  `gorm:"primaryKey;autoIncrement:false"`
	CodigoTerritorial int    `gorm:"index"`
	RecintoNombre     string `gorm:"size:150"`
	RecintoDireccion  string `gorm:"type:text"`
	NumMesas          int
	Territorio        DistribucionTerritorial `gorm:"foreignKey:CodigoTerritorial;references:CodigoTerritorial;constraint:-"`
}

func (RecintoElectoral) TableName() string {
	return "recinto_electoral"
}

type Mesa struct {
	CodigoActa          int64 `gorm:"primaryKey;autoIncrement:false"`
	CodigoRecinto       int64 `gorm:"index"`
	NroMesa             int
	VotantesHabilitados int
	Recinto             RecintoElectoral `gorm:"foreignKey:CodigoRecinto;references:CodigoRecinto;constraint:-"`
}

func (Mesa) TableName() string {
	return "mesas"
}

type PartidoPolitico struct {
	IdPartido int    `gorm:"primaryKey"`
	Sigla     string `gorm:"size:20"`
}

func (PartidoPolitico) TableName() string {
	return "partido_politico"
}

type Usuario struct {
	IdUsuario int    `gorm:"primaryKey"`
	Nombre    string `gorm:"size:100;uniqueIndex"`
}

func (Usuario) TableName() string {
	return "usuarios"
}

type Papeleta struct {
	IdPapeleta          int `gorm:"primaryKey"`
	CodigoActa          int64 `gorm:"uniqueIndex"`
	VotosValidos        int
	VotosBlancos        int
	VotosNulos          int
	PapeletasAnfora     int
	PapeltasNoUtilizadas int
	AperturaHora        int
	AperturaMinutos     int
	CierreHora          int
	CierreMinutos       int
	Observaciones       string `gorm:"type:text"`
	IdUsuario           int
	FechaCreacion       time.Time `gorm:"autoCreateTime"`
	
	Mesa    Mesa    `gorm:"foreignKey:CodigoActa;references:CodigoActa;constraint:-"`
	Usuario Usuario `gorm:"foreignKey:IdUsuario;references:IdUsuario;constraint:-"`
}

func (Papeleta) TableName() string {
	return "papeleta"
}

type DetalleVotosPartido struct {
	IdDetalle     int `gorm:"primaryKey"`
	IdPapeleta    int `gorm:"index"`
	IdPartido     int `gorm:"index"`
	CantidadVotos int
	
	Papeleta Papeleta `gorm:"foreignKey:IdPapeleta;references:IdPapeleta;constraint:-"`
	Partido  PartidoPolitico `gorm:"foreignKey:IdPartido;references:IdPartido;constraint:-"`
}

func (DetalleVotosPartido) TableName() string {
	return "detalle_votos_partido"
}

// CSV Structs mapping
type CSVTerritorio struct {
	CodigoTerritorial int    `csv:"CodigoTerritorial"`
	Departamento      string `csv:"Departamento"`
	Municipio         string `csv:"Municipio"`
	Provincia         string `csv:"Provincia"`
}

type CSVRecinto struct {
	RecintoCode       string `csv:"recintoCode"`
	CodigoTerritorial int    `csv:"CodigoTerritorial"`
	CodigoRecinto     int64  `csv:"CodigoRecinto"`
	RecintoNombre     string `csv:"RecintoNombre"`
	RecintoDireccion  string `csv:"RecintoDireccion"`
	NumMesas          int    `csv:"NumMesas"`
}

type CSVActa struct {
	CodigoRecinto       int64 `csv:"CodigoRecinto"`
	CodigoActa          int64 `csv:"CodigoActa"`
	NroMesa             int   `csv:"NroMesa"`
	VotantesHabilitados int   `csv:"VotantesHabilitados"`
}
