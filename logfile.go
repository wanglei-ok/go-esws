/*
Copyright 2018 The go-eam Authors
This file is part of the go-eam library.

logfile
设置标准日志的输出格式并输出到文件


wanglei.ok@foxmail.com

1.0
版本时间：2018年4月13日18:32:12

*/

package main

import (
	"os"
	"io"
	"path/filepath"
	"strings"
	"fmt"
	"time"
	"log"
)


func init(){
	logSetup()
}


//取得当前可执行程序路径
func getCurrentDirectory() string {
	dir, err := filepath.Abs(filepath.Dir(os.Args[0]))
	if err != nil {
		log.Fatal(err)
	}
	return strings.Replace(dir, "\\", "/", -1)
}

//创建path执行的文件夹
func createDir(path string) bool {
	// check
	if _, err := os.Stat(path); err != nil {
		//不存在创建
		err := os.MkdirAll(path, 0711)

		if err != nil {
			return false
		}
	}
	return true
}

//取得当前可执行程序名称
func baseName() string {
	return filepath.Base(os.Args[0])
}

//在当前可执行程序路径下创建log文件夹存放日志
func createLogDir() string {
	//当前目录加log
	logPath := fmt.Sprintf("%s/log", getCurrentDirectory())
	if createDir(logPath) {
		return logPath
	}
	return  ""
}


type myWriter struct {
	createdDate string
	file *os.File
}


func (t *myWriter) Write(p []byte) (n int, err error) {

	tt := string(p[5:10])
	if t.createdDate != tt {
		if err := t.rotateFile(time.Now()); err != nil {
			log.Println(err)
		}
	}

	return t.file.Write(p)
}

//分割日志文件
func (t *myWriter) rotateFile(now time.Time) error {
	t.createdDate = fmt.Sprintf("%02d/%02d", now.Month(), now.Day())
	//创建新的日志文件
	logDir := createLogDir()
	if len(logDir) != 0 {
		//baseName_YYYYMMDD.log
		//格式化方式为 /path/to/file/<prefix>YYYYMMDD<suffix>
		filePath := fmt.Sprintf("%s/%s%s%s", logDir, baseName()+"_", now.Format("20060102"), ".log")

		file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err != nil {
			return err
		}

		if t.file != nil {
			t.file.Close()
		}
		t.file = file
		//
	}
	return nil
}

//设置默认logger的
//标志位log.Ldate | log.Lmicroseconds
//设置日志输出到文件 ./log/baseName_YYYYMMDD.log
func logSetup() {
	log.SetFlags(log.Ldate | log.Lmicroseconds |log.Lshortfile)
	log.SetOutput(io.MultiWriter(&myWriter{}, os.Stderr))
}
