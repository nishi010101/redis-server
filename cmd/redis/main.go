package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

var data = make(map[string]dataVal)

//var data = sync.Map{}

var dataMutex = sync.RWMutex{}

var conflictingOptions = map[string][][]string{
	"SET": {
		{"NX", "XX"},
		{"EX", "PX", "EXAT", "PXAT", "KEEPTTL"},
	},
}

type dataVal struct {
	Value      interface{}
	ExpiryTime *time.Time
}

type Option struct {
	name  string
	value *interface{}
}

func main() {

	var wg sync.WaitGroup

	// Set up a channel to receive signals
	signalChannel := make(chan os.Signal, 1)
	signal.Notify(signalChannel, os.Interrupt, syscall.SIGTERM)

	// Perform cleanup when a signal is received
	go func() {
		sig := <-signalChannel
		fmt.Printf("Received signal: %v\n", sig)
		fmt.Println("Saving the final RDB snapshot before exiting")
		err := saveDataOnDisk()
		if err != nil {
			fmt.Println("oopsie, error while saving data to disk")
		}
		wg.Wait()
		fmt.Println("Redis is now ready to exit, bye bye...")
		os.Exit(1)
	}()

	fileData, err := os.ReadFile("backUp.rdb")
	if err != nil {
		file, err := os.Create("backUp.rdb")
		if err != nil {
			return
		}
		defer file.Close()
	} else {
		dataFromDisk := make(map[string]dataVal)
		if len(fileData) > 0 {
			err := json.Unmarshal(fileData, &dataFromDisk)
			if err != nil {
				return
			}
			data = dataFromDisk
			fmt.Printf("%v keys retrieved from disk\n", len(data))
		}
	}

	listener, err := net.Listen("tcp", "0.0.0.0:6349")
	if err != nil {
		println("error while listening")
		return
	}
	defer listener.Close()

	go updateDataByTTL() //active deletion of key

	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Println("Error accepting connection:", err)
			continue
			// handle error
		}
		go handleConnection(&conn)
	}

}

func saveDataOnDisk() error {
	dataMutex.RLock()
	jsonData, err := json.Marshal(data)
	if err != nil {
		log.Fatalf("failed to marshal: %v", err)
	}
	dataMutex.RUnlock()
	fmt.Println("DB saved on disk")
	return os.WriteFile("backUp.rdb", jsonData, 0644)
}

func updateDataByTTL() {
	for {
		time.Sleep(1 * time.Second)
		updateDataByTTL1()
	}
}

const dataCountUndertest = 2

func updateDataByTTL1() {

	var expiringRatio float32 = 100
	for expiringRatio > 25 {
		dataMutex.RLock()
		count := dataCountUndertest
		var keys []string
		for key, val := range data {
			if val.ExpiryTime != nil {
				keys = append(keys, key)
			}
		}
		if count > len(keys) {
			count = len(keys)
		}
		dataMutex.RUnlock()
		var expiringCount float32 = 0
		if len(keys) == 0 {
			break
		}
		randomKeys := getRandomKeys(keys, count)
		if len(randomKeys) != 0 {
			dataMutex.RLock()
			for _, randomKey := range randomKeys {
				if data[randomKey].ExpiryTime != nil && (*(data[randomKey].ExpiryTime)).Before(time.Now()) {
					fmt.Printf("deleting key %s since it expired at %s ", randomKey, *(data[randomKey].ExpiryTime))
					fmt.Println()
					delete(data, randomKey)
					expiringCount++
				}
			}
			dataMutex.RUnlock()
			expiringRatio = (expiringCount / float32(len(randomKeys))) * 100
		} else {
			expiringRatio = 0
		}
	}
	return
}

func getRandomKeys(keys []string, count int) []string {
	for i := range keys {
		j := rand.Intn(i + 1)
		keys[i], keys[j] = keys[j], keys[i]
	}
	var randomkeys []string
	for i := 0; i < count; i++ {
		randomkeys = append(randomkeys, keys[i])
	}
	return randomkeys
}

func handleConnection(conn *net.Conn) {
	defer (*conn).Close()

LOOP:
	for {
		//println("inside for loop")
		//println("----------")

		byteRead := make([]byte, 1024)
		_, err := (*conn).Read(byteRead)
		if err != nil {
			return
		}

		byteString := string(byteRead)
		command, arguments := deserialise(&byteString)
		//fmt.Println("command recieved ", command)
		//fmt.Println("arguments recieved ", arguments)
		var respString string
		switch strings.ToUpper(command) {
		case "PING":
			respString = "PONG"
			writeResponse(*conn, respString)
		case "ECHO":
			for key, val := range *arguments {
				respString = respString + val
				if key < len(*arguments) {
					respString = respString + " "
				}
			}
			writeResponse(*conn, respString)
		case "SET":
			var key string
			var value string
			var options []Option

			index := 0
			for arguments != nil && index < len(*arguments) {
				if index == 0 {
					key = (*arguments)[index]
					index++
					continue
				}
				if index == 1 {
					value = (*arguments)[index]
					index++
					continue
				}

				if index > 1 {
					if (*arguments)[index] == "NX" || (*arguments)[index] == "XX" || (*arguments)[index] == "KEEPTTL" {
						options = append(options, Option{
							name:  (*arguments)[index],
							value: nil,
						})
						index++
					} else if (*arguments)[index] == "EX" || (*arguments)[index] == "PX" || (*arguments)[index] == "EXAT" || (*arguments)[index] == "PXAT" {
						optionValIndex := index + 1
						if optionValIndex >= len(*arguments) {
							respString = fmt.Sprintf("No value given for option %s.", (*arguments)[index])
							writeResponse(*conn, respString)
							goto LOOP
						} else {
							var myInterface interface{} = (*arguments)[index+1]
							options = append(options, Option{
								name:  (*arguments)[index],
								value: &myInterface,
							})
							index = index + 2
						}

					} else {
						respString = fmt.Sprintf("Invalid option %s.", (*arguments)[index])
						writeResponse(*conn, respString)
						goto LOOP
					}
				}
			}

			if value == "" {
				respString = fmt.Sprintf("VAL for %s is not valid.", key)
				writeResponse(*conn, respString)
				goto LOOP
			}

			var expiryTime *time.Time
			if optionsAreNotConflicting(command, options) {
				//fmt.Println("command", command)
				//fmt.Println("value", value)
				//fmt.Println("options", options)
				for _, val := range options {
					switch val.name {
					case "NX":
						_, exists := data[key]
						if exists {
							writeResponse(*conn, fmt.Sprintf("oopsie value already exists, so not setting"))
							goto LOOP
						}
					case "XX":
						_, exists := data[key]
						if !exists {
							writeResponse(*conn, fmt.Sprintf("oopsie value does not exist, so not setting"))
							goto LOOP
						}
					case "EX":
						expiryInSeconds := fmt.Sprintf("%v", *val.value)
						expiryInSecondsInt, err := strconv.Atoi(expiryInSeconds)
						if err != nil {
							writeResponse(*conn, fmt.Sprintf("VAL for %s is not valid.", expiryInSeconds))
						}
						time1 := time.Now().Add(time.Duration(expiryInSecondsInt) * time.Second)
						expiryTime = &time1
					case "PX":
						expiryInSeconds := fmt.Sprintf("%v", *val.value)
						expiryInSecondsInt, err := strconv.Atoi(expiryInSeconds)
						if err != nil {
							writeResponse(*conn, fmt.Sprintf("VAL for %s is not valid.", expiryInSeconds))
							goto LOOP
						}
						time1 := time.Now().Add(time.Duration(expiryInSecondsInt) * time.Millisecond)
						expiryTime = &time1
					case "EXAT":
						unixSec := fmt.Sprintf("%v", *val.value)
						unixSecInt, err := strconv.ParseInt(unixSec, 10, 64)
						if err != nil {
							writeResponse(*conn, fmt.Sprintf("VAL for %s is not valid.", unixSec))
							goto LOOP
						}
						time1 := time.Unix(unixSecInt, 0)
						expiryTime = &time1
					case "PXAT":
						unixMilliSec := fmt.Sprintf("%v", *val.value)
						unixMilliSecInt, err := strconv.ParseInt(unixMilliSec, 10, 64)
						if err != nil {
							writeResponse(*conn, fmt.Sprintf("VAL for %s is not valid.", unixMilliSec))
							goto LOOP
						}
						time1 := time.UnixMilli(unixMilliSecInt)
						expiryTime = &time1
					case "KEEPTTL":
						dataMutex.RLock()
						val, exists := data[key]
						if exists {
							expiryTime = val.ExpiryTime
						}
						dataMutex.RUnlock()

					}

				}

				go func() {
					dataMutex.Lock()
					if expiryTime != nil {
						data[key] = dataVal{
							Value:      value,
							ExpiryTime: expiryTime,
						}
					} else {
						data[key] = dataVal{
							Value:      value,
							ExpiryTime: nil,
						}
					}
					dataMutex.Unlock()
				}()
				writeResponse(*conn, "OK")

			} else {
				writeResponse(*conn, "oopsie conflicting options")
			}

		case "GET":
			dataMutex.RLock()
			datakey := (*arguments)[0]
			dataValue := data[datakey]
			dataMutex.RUnlock()
			if dataValue.ExpiryTime != nil {
				if (*dataValue.ExpiryTime).After(time.Now()) {
					if stringVal, ok := data[datakey].Value.(string); ok {
						respString = stringVal
					}
				} else { // passive deletion of key
					dataMutex.Lock()
					delete(data, datakey)
					dataMutex.Unlock()
				}
			} else {
				if stringVal, ok := data[datakey].Value.(string); ok {
					respString = stringVal
				}
			}

			if respString == "" {
				respString = "<nil>"
			}
			writeResponse(*conn, respString)
		case "EXISTS":
			dataMutex.RLock()
			keyExists := 0
			for _, val := range *arguments {
				_, exists := data[val]
				if exists {
					keyExists++
				}
			}
			dataMutex.RUnlock()
			respString = strconv.Itoa(keyExists)
			writeResponse(*conn, respString)
		case "DEL":
			dataMutex.Lock()
			deletedKeys := 0
			for _, val := range *arguments {
				_, exists := data[val]
				if exists {
					delete(data, val)
					deletedKeys++
				}
			}
			dataMutex.Unlock()
			respString = strconv.Itoa(deletedKeys)
			writeResponse(*conn, respString)
		case "INCR":
			dataMutex.Lock()
			val, exists := data[(*arguments)[0]] // what happens if we paas more than one keys
			if exists {
				if valInt, err := strconv.ParseInt(val.Value.(string), 10, 64); err == nil {
					respString = strconv.FormatInt(valInt+1, 10)
					data[(*arguments)[0]] = dataVal{Value: respString, ExpiryTime: val.ExpiryTime}
				} else {
					respString = fmt.Sprintf("Cant increment! Not an Int Value.")
					writeResponse(*conn, respString)
					goto LOOP
				}
			}
			dataMutex.Unlock()
			writeResponse(*conn, respString)
		case "DECR":
			dataMutex.Lock()
			val, exists := data[(*arguments)[0]] // what happens if we paas more than one keys
			if exists {
				if valInt, err := strconv.ParseInt(val.Value.(string), 10, 64); err == nil {
					respString = strconv.FormatInt(valInt-1, 10)
					data[(*arguments)[0]] = dataVal{Value: respString, ExpiryTime: val.ExpiryTime}
				} else {
					respString = fmt.Sprintf("Cant increment! Not an Int Value.")
					writeResponse(*conn, respString)
					goto LOOP
				}
			}
			dataMutex.Unlock()
			writeResponse(*conn, respString)
		case "LPUSH":
			dataMutex.Lock()
			listKey := (*arguments)[0]
			listVal, exists := data[listKey]
			elements := (*arguments)[1:] // what happens if we DONT PASS ANY ELEMNTS
			if exists {
				reverseSlice(elements)
				list, ok := listVal.Value.([]string)
				if ok {
					finalList := append(elements, list...)
					data[listKey] = dataVal{
						Value:      finalList,
						ExpiryTime: nil,
					}
					respString = strconv.Itoa(len(finalList))
				} else {
					writeResponse(*conn, fmt.Sprintf("oopsie cant push, not a list"))
					goto LOOP
				}
			} else {
				data[listKey] = dataVal{
					Value:      elements,
					ExpiryTime: nil,
				}
				respString = strconv.Itoa(len(elements))
			}
			dataMutex.Unlock()
			writeResponse(*conn, respString)
		case "RPUSH":
			dataMutex.Lock()
			listKey := (*arguments)[0]
			listVal, exists := data[listKey] // what happens if we DONT PASS ANY ELEMNTS
			elements := (*arguments)[1:]
			if exists {
				list, ok := listVal.Value.([]string)
				if ok {
					finalList := append(list, elements...)
					data[listKey] = dataVal{
						Value:      finalList,
						ExpiryTime: nil,
					}
					respString = strconv.Itoa(len(finalList))
				} else {
					writeResponse(*conn, fmt.Sprintf("oopsie cant push, not a list"))
					goto LOOP
				}
			} else {
				data[listKey] = dataVal{
					Value:      elements,
					ExpiryTime: nil,
				}
				respString = strconv.Itoa(len(elements))
			}
			dataMutex.Unlock()
			writeResponse(*conn, respString)
		case "SAVE":
			err := saveDataOnDisk()
			if err != nil {
				writeResponse(*conn, fmt.Sprintf("Unable to save data on disk"))
				goto LOOP
			}
			fmt.Println("Saving data to RDB")
			writeResponse(*conn, "OK")
		default:
			writeResponse(*conn, "Unknown command")
		}
	}

}

func reverseSlice(slice []string) {
	// Get the length of the slice
	length := len(slice)

	// Reverse the order of elements in the slice
	for i, j := 0, length-1; i < j; i, j = i+1, j-1 {
		// Swap elements at indices i and j
		slice[i], slice[j] = slice[j], slice[i]
	}
}

func optionsAreNotConflicting(command string, options []Option) bool {
	optionsAreNotConflicting := true
	conflictingOptionsForCommand := conflictingOptions[command]
	for _, val := range conflictingOptionsForCommand {
		count := 0
		for _, option := range val {
			if contains(options, option) {
				count++
			}

			if count > 1 {
				optionsAreNotConflicting = false
				break
			}
		}
	}

	return optionsAreNotConflicting
}

func contains(slice []Option, value string) bool {
	valExists := false
	for _, val := range slice {
		if val.name == value {
			valExists = true
			break
		}
	}
	return valExists
}

func deserialise(data *string) (string, *[]string) {
	firstIndex := strings.Index(*data, "\r\n")
	mainString := (*data)[firstIndex+2:]
	vals := strings.Split(mainString, "\r\n")
	command := ""
	var arguments []string
	for key, val := range vals {
		if key == 1 {
			command = val
		} else if key != 0 && key%2 != 0 {
			arguments = append(arguments, val)
		}
	}
	//fmt.Println("command", command)
	//fmt.Println("arguments")
	//for _, val := range arguments {
	//	fmt.Println(val)
	//}
	return command, &arguments

}

func writeResponse(conn net.Conn, data string) {
	serialisedVal := *serialise(data)
	serialisedValBytes := []byte(serialisedVal)
	conn.Write(serialisedValBytes)
}

func serialise(data interface{}) *string {

	dataType := fmt.Sprintf("%T", data)
	var serialisedData string
	switch dataType {
	case "string":
		serialisedData = "+" + data.(string) + "\r\n"
	case "*errors.errorString":
		serialisedData = "-" + data.(error).Error() + "\r\n"
	case "int":
		serialisedData = ":" + strconv.Itoa(data.(int)) + "\r\n"
	case "[]string":
		dataVal := data.([]string)
		serialisedData = "*" + strconv.Itoa(len(dataVal)) + "\r\n"
		for _, val := range dataVal {
			serialisedData = serialisedData + "$" + strconv.Itoa(len(val)) + "\r\n" + val + "\r\n"
		}
	}

	if serialisedData == "" {
		return nil
	}
	return &serialisedData
}
