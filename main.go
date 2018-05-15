/*
Copyright 2018 The go-eam Authors
This file is part of the go-eam library.

main
这个例子用以太坊etherscan Websocket接口，用于监控某个账号的交易事件。


wanglei.ok@foxmail.com

1.0
版本时间：2018年4月13日18:32:12

*/

package main

import (
	"net/url"
	"os"
	"os/signal"
	"time"
	"log"
	"github.com/gorilla/websocket"
	"github.com/pkg/errors"
	"encoding/json"
	"bytes"
	"strings"
	"strconv"
	"github.com/wanglei-ok/logfile"
)


var (
	eais map[string]int
)

type EtherscanWS struct{
	url url.URL
}

func NewEtherscanWS() *EtherscanWS{
	return &EtherscanWS{
		url:url.URL{Scheme: "wss", Host: "socket.etherscan.io", Path: "/wshandler"},
	}
}

func (e * EtherscanWS)start() error {
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	log.Printf("Connecting to %s", e.url.String())

	//连接
	c, _, err := websocket.DefaultDialer.Dial(e.url.String(), nil)
	if err != nil {
		log.Println("Dial:", err)
		return err
	}
	defer c.Close()

	done := make(chan struct{})

	//启动接收历程
	go func() {
		defer close(done)
		for {
			_, message, err := c.ReadMessage()
			if err != nil {
				log.Println("Read:", err)
				return
			}
			log.Printf("Recv: %s", message)
			var txlistJson TxlistJson
			err = json.NewDecoder(bytes.NewBuffer(message)).Decode(&txlistJson)
			if err != nil {
				log.Println("Decode:", err)
				continue
			}

			if txlistJson.Event == "txlist" {
				//没有交易记录
				if txlistJson.Result == nil {
					continue
				}

				addr := txlistJson.Address
				startBlock,ok := eais[addr]
				if !ok {
					continue
				}

				maxBlock := startBlock

				proc := 0
				inc := 0
				skip := 0

				//遍历交易记录插入数据库
				//开启事务
				trans, err := TxBegin()
				if err != nil {
					log.Println("Error TxBegin:", err.Error())
					continue
				}
				for _ , tx := range txlistJson.Result {
					proc++
					err := trans.InsertTx(&tx)
					if err != nil {
						//插入失败显示日志
						//txString, _ := json.Marshal(tx)
						errString := err.Error()
						if strings.Contains(errString, "Duplicate entry") {
							log.Printf("Skip Duplicate tx: %s%s", config.EtherscanApi.ApiTx,tx.Hash)
						} else {
							log.Printf("Error insertTx:%v, %s%s", errString, config.EtherscanApi.ApiTx, tx.Hash)
						}
						skip++
					}else{
						inc++
						log.Printf("Increase tx: %s%s", config.EtherscanApi.ApiTx,tx.Hash)
					}

					//最后块编号
					b, ok := strconv.Atoi(tx.BlockNumber )
					if ok == nil {
						if b > maxBlock {
							maxBlock = b
						}
					}
				}

				//保存最后块编号
				if maxBlock > startBlock {
					if trans.UpdateLastBlock(addr, maxBlock) == 0 {
						trans.Rollback()
					}
				}
				trans.Commit()
			} else if txlistJson.Event == "subscribe-txlist" {
				if txlistJson.Status ==  "1" {
					//订阅成功检索一次历史数据
					addr := txlistJson.Message[4:]
					startBlock,ok := eais[addr]
					if ok {
						retrieve(addr, startBlock)
					}
				}
			}
		}
	}()

	pingTicker := time.NewTicker(time.Second*20)
	defer pingTicker.Stop()

	//取得地址和最后一次更新的块号
	eais, err = GetEthAddress()
	if err != nil {
		log.Println("Error GetEthAddress:", err.Error())
		return err
	}

	if len(eais) == 0 {
		log.Println("Error not found ethereum address.")
		return err
	}
	for a,_ := range(eais) {
		//订阅交易记录
		err = c.WriteMessage(websocket.TextMessage, []byte("{\"event\": \"txlist\", \"address\": \""+a+"\"}"))
		if err != nil {
			log.Println("Subscribe write:", err)
			//订阅错误输出不退出
		}

	}

	//发送Ping直到连接断开
	//或者用户中断
	for {
		select {
		case <-done:
			return errors.New("Restart..")
		case <-pingTicker.C:
			err := c.WriteMessage(websocket.TextMessage, []byte("{\"event\":\"ping\"}"))
			if err != nil {
				log.Println("Ping write:", err)
				return err
			}
		case <-interrupt: // 用户中断自动停止
			log.Println("Interrupt")

			// Cleanly close the connection by sending a close message and then
			// waiting (with timeout) for the server to close the connection.
			err := c.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			if err != nil {
				log.Println("Write close:", err)
				return err
			}
			select {
			case <-done:
			case <-time.After(time.Second):
			}
			return nil
		}
	}
}

func (e * EtherscanWS)Run() {
	for {
		if err := e.start(); err == nil {
			break
		}

		log.Println("The network is disconnected and reconnected after 10 seconds.")
		//等一段时间
		//重新连接
		time.Sleep(time.Second*10)
	}
}


//取回指定地址和开始块号
func retrieve(addr string, startBlock int) {
	proc := 0
	inc := 0
	skip := 0

	t1 := time.Now()
	log.Printf("Begin retrieve.(Address: %s%s, StartBlock:%d)\n", config.EtherscanApi.ApiAddress, addr, startBlock)
	defer func (){
		elapsed := time.Since(t1)
		log.Printf("End retrieve.(Elapsed:%v, Process:%d, Increase:%d, Skip:%d)\n", elapsed, proc, inc, skip)
	}()

	maxBlock := startBlock

	//使用Etherscan API检索交易列表
	txlistJson, err := Retrieve(addr, startBlock, true)
	//检索失败处理
	if err != nil {
		log.Println("Error retrieve:", err.Error())
		return
	}

	//API返回错误处理
	if txlistJson.Status != "1" {
		log.Println("Etherscan api:", txlistJson.Message)
		return
	}

	//没有交易记录
	if txlistJson.Result == nil {
		return
	}

	//遍历交易记录插入数据库
	//开启事务
	trans, err := TxBegin()
	if err != nil {
		log.Println("Error TxBegin:", err.Error())
		return
	}
	for _ , tx := range txlistJson.Result {
		proc++
		err := trans.InsertTx(&tx)
		if err != nil {
			//插入失败显示日志
			//txString, _ := json.Marshal(tx)
			errString := err.Error()
			if strings.Contains(errString, "Duplicate entry") {
				log.Printf("Skip Duplicate tx: %s%s", config.EtherscanApi.ApiTx,tx.Hash)
			} else {
				log.Printf("Error insertTx:%v, %s%s", errString, config.EtherscanApi.ApiTx, tx.Hash)
			}
			skip++
		}else{
			inc++
			log.Printf("Increase tx: %s%s", config.EtherscanApi.ApiTx,tx.Hash)
		}

		//最后块编号
		b, ok := strconv.Atoi(tx.BlockNumber )
		if ok == nil {
			if b > maxBlock {
				maxBlock = b
			}
		}
	}

	//保存最后块编号
	if maxBlock > startBlock {
		if trans.UpdateLastBlock(addr, maxBlock) == 0 {
			trans.Rollback()
		}
	}

	trans.Commit()
}

func init(){
	logfile.Setup()
}

//每十分钟运行一次
//查询最后更新块到最新块的交易记录
//交易记录写入数据库
func main() {

	if err := readConfig(); err != nil {
		log.Println("Error readConfig:", err)
		return
	}


	//打开数据库
	if err := OpenDatabase(config.Mysql.DSN); err != nil {
		log.Println("Error OpenDatabase:", err.Error())
		return
	}
	//程序结束前关闭数据库
	defer CloseDatabase()

	ews := NewEtherscanWS()
	ews.Run()
}