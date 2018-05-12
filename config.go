package main

import (
	"gopkg.in/gcfg.v1"
	"path/filepath"
	"os"
	"strings"
)

var config Config

type (
	Config struct{
		Mysql struct{
			DSN string
		}

		EtherscanApi struct {
			ApiTxlist string
			ApiAddress string
			ApiTx string
		}
	}
)

func iniFileName() string {
	exePath := os.Args[0]
	base := filepath.Base(exePath)
	suffix := filepath.Ext(exePath)
	return strings.TrimSuffix(base, suffix) + ".ini"
}

func readConfig() error {
	return gcfg.ReadFileInto(&config, iniFileName())
}