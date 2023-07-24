package main

import "time"

func main() {
	go ServerGinBefore()
	go CheckAddressGinBefore()
	time.Sleep(time.Hour)
}
