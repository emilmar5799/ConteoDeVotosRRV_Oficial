-- Creación de la base de datos (opcional si ya la creaste)
-- CREATE DATABASE sistema_electoral;

-- 1. Tabla: Usuarios
CREATE TABLE Usuarios (
    id_usuario SERIAL PRIMARY KEY,
    nombre VARCHAR(100) NOT NULL,
    rol VARCHAR(50),
    fecha_creacion TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    usuario_creacion VARCHAR(50),
    fecha_modificacion TIMESTAMP,
    usuario_modificacion VARCHAR(50)
);

-- 2. Tabla: Distribucion_Territorial
CREATE TABLE Distribucion_Territorial (
    id_territorio SERIAL PRIMARY KEY,
    departamento VARCHAR(100) NOT NULL,
    provincia VARCHAR(100),
    municipio VARCHAR(100),
    fecha_creacion TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    usuario_creacion VARCHAR(50),
    fecha_modificacion TIMESTAMP,
    usuario_modificacion VARCHAR(50)
);

-- 3. Tabla: Partido_Politico
CREATE TABLE Partido_Politico (
    id_party SERIAL PRIMARY KEY, -- En el diagrama dice id_partido
    nombre VARCHAR(100) NOT NULL,
    sigla VARCHAR(20),
    fecha_creacion TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    usuario_creacion VARCHAR(50),
    fecha_modificacion TIMESTAMP,
    usuario_modificacion VARCHAR(50)
);

-- 4. Tabla: Recinto_Electoral
CREATE TABLE Recinto_Electoral (
    id_recinto SERIAL PRIMARY KEY,
    nombre VARCHAR(150) NOT NULL,
    direccion TEXT,
    id_territorio INT,
    fecha_creacion TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    usuario_creacion VARCHAR(50),
    fecha_modificacion TIMESTAMP,
    usuario_modificacion VARCHAR(50),
    CONSTRAINT fk_territorio FOREIGN KEY (id_territorio) 
        REFERENCES Distribucion_Territorial(id_territorio)
);

-- 5. Tabla: Mesas
CREATE TABLE Mesas (
    id_mesa SERIAL PRIMARY KEY,
    nro_mesa INT NOT NULL,
    cantidad_habilitados INT,
    id_recinto INT,
    fecha_creacion TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    usuario_creacion VARCHAR(50),
    fecha_modificacion TIMESTAMP,
    usuario_modificacion VARCHAR(50),
    CONSTRAINT fk_recinto FOREIGN KEY (id_recinto) 
        REFERENCES Recinto_Electoral(id_recinto)
);

-- 6. Tabla: Papeleta
-- Nota: id_mesa tiene un constraint UNIQUE según tu diagrama (relación 1 a 1 aparente)
CREATE TABLE Papeleta (
    id_papeleta SERIAL PRIMARY KEY,
    id_mesa INT UNIQUE,
    cantidad_anfora INT,
    papeletas_no_usadas INT,
    votos_validos INT,
    votos_nulos INT,
    votos_blancos INT,
    fecha_creacion TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    usuario_creacion VARCHAR(50),
    fecha_modificacion TIMESTAMP,
    usuario_modificacion VARCHAR(50),
    CONSTRAINT fk_mesa FOREIGN KEY (id_mesa) 
        REFERENCES Mesas(id_mesa)
);

-- 7. Tabla: Detalle_Votos_Partido
CREATE TABLE Detalle_Votos_Partido (
    id_detalle SERIAL PRIMARY KEY,
    id_papeleta INT,
    id_partido INT,
    cantidad_votos INT,
    fecha_creacion TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    usuario_creacion VARCHAR(50),
    CONSTRAINT fk_papeleta FOREIGN KEY (id_papeleta) 
        REFERENCES Papeleta(id_papeleta),
    CONSTRAINT fk_partido FOREIGN KEY (id_partido) 
        REFERENCES Partido_Politico(id_party)
);