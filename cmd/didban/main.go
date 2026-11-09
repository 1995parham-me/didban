// Command didban is a home-monitoring daemon for a Raspberry Pi sensor HAT:
// it reads the I²C sensors (BMP180, DS3231, PCF8591), exports Prometheus
// metrics, and drives a PCF8574-backed HD44780 LCD.
package main

import "github.com/1995parham-me/didban/internal/cmd"

func main() {
	cmd.Execute()
}
