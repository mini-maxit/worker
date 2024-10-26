package models

import (
    "time"
)

type Task struct {
    ID           uint   `gorm:"primaryKey;autoIncrement"`
    Title        string `gorm:"type:varchar(255);not null"`
    DirPath      string `gorm:"type:varchar(255);not null"`
    InputDirPath string `gorm:"type:varchar(255);not null"`
    OutputDirPath string `gorm:"type:varchar(255);not null"`
}

type UserSolution struct {
    ID               uint      `gorm:"primaryKey;autoIncrement"`
    TaskID           uint      `gorm:"not null; foreignKey:TaskID"`
    SolutionFileName string    `gorm:"type:varchar(255);not null"`
    LanguageType     string    `gorm:"type:varchar(255);not null"`
    LanguageVersion  string    `gorm:"type:varchar(50);not null"`
    Status           string    `gorm:"type:varchar(50);not null"`
    SubmittedAt      time.Time `gorm:"autoCreateTime"`
    CheckedAt        *time.Time
    StatusMessage    string    `gorm:"type:varchar"`
}

type InputOutput struct {
    ID          uint `gorm:"primaryKey"`
    TaskID      uint    `gorm:"not null: foreignKey:TaskID"`
    InputOutputOrder       int     `gorm:"not null"`
    TimeLimit   float64 `gorm:"not null"`
    MemoryLimit float64 `gorm:"not null"`
}

type UserSolutionResult struct {
    ID             uint      `gorm:"primaryKey;autoIncrement"`
    UserSolutionID uint      `gorm:"not null; foreignKey:UserSolutionID"`
    Code           string    `gorm:"not null"`
    Message        string    `gorm:"type:varchar(255);not null"`
    CreatedAt      time.Time `gorm:"autoCreateTime"`
}


type TestResult struct {
    ID                    uint   `gorm:"primaryKey;autoIncrement"`
    UserSolutionResultID   uint   `gorm:"not null; foreignKey:UserSolutionResultID"`
    InputOutputID          uint    `gorm:"not null; foreignKey:InputOutputID"`
    OutputFilePath         string `gorm:"type:varchar;not null"`
    Passed                 bool   `gorm:"not null"`
    ErrorMessage           string `gorm:"type:varchar"`
}
