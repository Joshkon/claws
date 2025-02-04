package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"golang.org/x/net/websocket"
	"io"
	"log"
	"net"
	"net/http"
	"time"
)

type MachineInfo struct{
	mac               string
	socket            net.Conn
	currentPlayer     net.Conn
	lastHeartbeat int64
}

var allMachines map[string]MachineInfo
var packetId int
var mainMachine = "22DA646AB4DE"
var gameActive = false

type RetGame struct{
	Command string `json:"command"`
	Result bool `json:"result"`
}

func main() {
	allMachines = map[string]MachineInfo{}

	fmt.Println("[GRIJPMACHINE] Gateway server opgestart, wachtend op verbinding.")

	go startBridge()
	go checkMachine()

	http.HandleFunc("/websocket",
		func (w http.ResponseWriter, req *http.Request) {
			s := websocket.Server{Handler: websocket.Handler(Echo)}
			s.ServeHTTP(w, req)
		})

	err := http.ListenAndServe(":8088",nil)
	if err != nil {
		log.Fatal("ListenAndServe:",err)
	}
}

func startBridge(){
	listener, _ := net.Listen("tcp", ":8080")

	for {
		conn, _ := listener.Accept()
		fmt.Println("[GRIJPMACHINE] Grijpmachine is verbonden met gateway server")
		go machineHandler(conn)
	}
}

func checkMachine(){
	for{
		time.Sleep(time.Duration(5)*time.Second)

		for k, v := range allMachines {
			if (time.Now().Unix() - v.lastHeartbeat) > 30 {
				fmt.Println("timeout remove", v.mac)
				delete(allMachines, k)
			}
		}
	}
}

func machineHandler(conn net.Conn){
	gCacheBuff := make([]byte, 4096)
	var curReadIndex int
	var curWriteIndex int
	var fillCount int

	buf := make([]byte, 4096)
	for {
		n,err := bufio.NewReader(conn).Read(buf)
		if err != nil {
			if err == io.EOF {
				fmt.Printf("client %s is close!\n",conn.RemoteAddr().String())
			}

			_ = conn.Close()
			return
		}

		len1 := n
		for iIndex := 0; iIndex < len1; iIndex++ {
			gCacheBuff[curWriteIndex] = buf[iIndex]
			fillCount++
			if fillCount >= 4096 {
				fmt.Println("com data out of buff ! warining! data will be overwrite lost!!!")
			}

			curWriteIndex++
			if curWriteIndex >= 4096{
				curWriteIndex = 0
			}
		}

		for{
			if fillCount <9 {
				break
			}

			if  gCacheBuff[curReadIndex] != 0xfe {
				fillCount--
				curReadIndex++
				if curReadIndex >= 4096{
					curReadIndex = 0
				}
				continue
			}

			if gCacheBuff[curReadIndex] == 0xfe {
				a0 := gCacheBuff[curReadIndex] & 0xff
				a3 := (^gCacheBuff[(curReadIndex+ 3) % 4096])& 0xff

				a1 := gCacheBuff[(curReadIndex+ 1) % 4096] & 0xff
				a4 := (^gCacheBuff[(curReadIndex+ 4) % 4096])& 0xff

				a2 := gCacheBuff[(curReadIndex+ 2) % 4096] & 0xff
				a5 := (^gCacheBuff[(curReadIndex+ 5) % 4096])& 0xff

				if (a0 != a3) || (a1 != a4) || (a2 != a5){
					fillCount--
					curReadIndex++
					if curReadIndex >= 4096{
						curReadIndex = 0
					}
					continue
				}

				len1 := gCacheBuff[(curReadIndex+ 6) % 4096]
				if fillCount < int(len1) {
					break
				}

				var sum int
				for kk := 6; kk < int(len1 - 1); kk++ {
					sum += int(gCacheBuff[(curReadIndex+ kk) % 4096])
				}
				sum = sum % 100
				if sum != int(gCacheBuff[(curReadIndex+ int(len1) - 1) % 4096]) {
					fillCount--
					curReadIndex++
					if  curReadIndex >= 4096{
						curReadIndex = 0
					}

					continue
				}

				cmd := gCacheBuff[(curReadIndex+ 7) % 4096] & 0xff
				if cmd == 0x35 {
					tmp := make([]byte, len1)
					mac := make([]byte, len1-9)
					for kk := 0; kk< int(len1); kk++{
						tmp[kk] = gCacheBuff[(curReadIndex+ kk)% 4096]
						if kk>=8 && kk<int(len1-1){
							mac[kk-8] = tmp[kk]
						}
					}
					_, _ = conn.Write(tmp)

					var str = string(mac[:])
					if _, ok := allMachines[str]; ok {
						t := allMachines[str]
						t.lastHeartbeat = time.Now().Unix()
						allMachines[str] = t
					}else{
						allMachines[str] = MachineInfo{mac: str, socket: conn, currentPlayer: allMachines[str].currentPlayer, lastHeartbeat:time.Now().Unix()}
					}
				} else if cmd == 0x31 {
					fmt.Println("[LOG] Game started")

					gameActive = true

					if allMachines[mainMachine].currentPlayer != nil {
						var rt = RetGame{Command: "start", Result: true}
						jsRet, err := json.Marshal(rt)
						if err != nil {
							fmt.Println("err 2", err)
						}

						_, _ = allMachines[mainMachine].currentPlayer.Write(jsRet)
					}
				} else if cmd == 0x33 {
					fmt.Println("[LOG] Game ended")

					gameActive = false

					if allMachines[mainMachine].currentPlayer != nil {
						hasWon := false
						if int(gCacheBuff[(curReadIndex+8)%4096]&0xff) == 1 {
							hasWon = true
						}

						var rt = RetGame{Command: "stop", Result: hasWon}
						jsRet, err := json.Marshal(rt)
						if err != nil {
							fmt.Println("err 2", err)
						}

						_, _ = allMachines[mainMachine].currentPlayer.Write(jsRet)
					}
				} else {
					var tmp[250] byte
					for kk := 0; kk< int(len1); kk++{
						tmp[kk] = gCacheBuff[(curReadIndex+ kk)% 4096]
					}
				}

				fillCount -= int(len1)
				curReadIndex = (curReadIndex + int(len1)) % 4096
			}
		}
	}
}

/*
	Gateway -> Bridge Handler
*/

func makeCom(nums ...byte) []byte {
	packLen := len(nums)+8

	packetId++

	pack := make([]byte, 7)
	pack[0] = 0xfe
	pack[1] = byte(packetId /256)
	pack[2] = byte(packetId %256)
	pack[3] = 1
	pack[4] = ^pack[1]
	pack[5] = ^pack[2]
	pack[6] = byte(packLen)
	pack = append(pack, nums...)

	sum := 0
	for i := 6;i<len(pack);i++ {
		sum += int(pack[i])
	}

	lastB := byte(sum%100)
	pack = append(pack, lastB)

	return pack
}

func Echo(ws *websocket.Conn) {
	var err error
	for {
		var reply string
		if err = websocket.Message.Receive(ws, &reply); err != nil {
			fmt.Println("receive failed:", err)
			break
		}

		fmt.Println("Received message")

		data2 := []byte(reply)

		var webData map[string]interface{}
		_ = json.Unmarshal(data2[:], &webData)
		strCmd := webData["command"]

		fmt.Println(strCmd)

		if strCmd == "start" {
			if gameActive {
				fmt.Println("[GRIJPMACHINE] Spel is al actief.")

				var rt = RetGame{Command:"start",Result:false}
				jsRet,err := json.Marshal(rt)
				if err != nil{
					fmt.Println("err 2", err)
				}

				_, _ = ws.Write(jsRet)
			} else {
				t1 := allMachines[mainMachine]
				t1.currentPlayer = ws
				allMachines[mainMachine] = t1

				gameTime := webData["gameTime"]
				letGrab := webData["letGrab"]
				grabPower := webData["grabPower"]
				topPower := webData["topPower"]
				movePower := webData["movePower"]
				maxPower := webData["maxPower"]
				topHeight := webData["topHeight"]
				lineLength := webData["lineLength"]

				xMotor := webData["xMotor"]
				zMotor := webData["zMotor"]
				yMotor := webData["yMotor"]

				comCmd := makeCom(0x31,byte(gameTime.(float64)),byte(letGrab.(float64)),byte(grabPower.(float64)),byte(topPower.(float64)),byte(movePower.(float64)),byte(maxPower.(float64)),byte(topHeight.(float64)),byte(lineLength.(float64)),byte(xMotor.(float64)),byte(zMotor.(float64)),byte(yMotor.(float64)))
				_, _ = allMachines[mainMachine].socket.Write(comCmd)
			}
		} else if strCmd == "grab" {
			comCmd := makeCom(0x32,4,136,19)
			_, _ = allMachines[mainMachine].socket.Write(comCmd)
		} else if strCmd == "move" {
			abc := byte(webData["action"].(float64))

			comCmd := makeCom(0x32,abc,136,19)
			_, _ = allMachines[mainMachine].socket.Write(comCmd)
		}
	}
}