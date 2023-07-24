package main

import (
	"database/sql"
	_ "github.com/go-sql-driver/mysql"
)

type AddressBefore struct {
	id    int
	value string
}

var dbBefore *sql.DB

func DBInitBefore() error {
	var err error
	dbBefore, err = sql.Open("mysql", "root:root@tcp(127.0.0.1:3306)/example")
	return err
}

func GetProvinceBefore(id string) string {
	var address AddressBefore
	err := dbBefore.QueryRow("select * from address where id = "+id).Scan(&address.id, &address.value)
	if err != nil {
		return err.Error()
	}
	return address.value
}

func GetCityBefore(id string) string {
	var address AddressBefore
	err := dbBefore.QueryRow("select * from address where id = "+id).Scan(&address.id, &address.value)
	if err != nil {
		return err.Error()
	}
	return address.value
}

func MockGetProvinceBefore(id string) string {
	return "SiChuan"
}

func MockGetCityBefore(id string) string {
	return "ChengDu"
}
