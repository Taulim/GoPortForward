package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"gopkg.in/yaml.v3"
)

// The main rule struct
type Rule struct {
	// Name does not have any effect on anything
	Name string `yaml:"Name"`

	// The port to listen on
	Listen uint16 `yaml:"Listen"`

	// The destination to forward the packets to
	Forward string `yaml:"Forward"`

	// Number of simultaneous connections allowed
	// 0 = No limit
	Simultaneous int `yaml:"Simultaneous"`
}

// The config file struct
type Config struct {
	// Default timeout
	// 0 = Disabled
	Timeout int64 `yaml:"Timeout"`

	// All of the forwarding rules
	Rules []Rule `yaml:"Rules"`
}

// A struct to count the active connections "Hello, world!"on each port
type CSafeConnections struct {
	Count []int
	mu    sync.RWMutex
}

// The config file name that has the default of rules.json
var ConfigFileName = "rules.yaml"

// Thread safe simultaneous connections
var MultiConn CSafeConnections

// Log level
var Verbose = 1

// The version of program
const Version = "0.0.1"

// Connection rules
var ConnRules []Rule

// Connection timeout
var ConnTimeout time.Duration

func main() {
	// Parse arguments
	{
		flag.StringVar(&ConfigFileName, "config", ConfigFileName, "Configuration file path")
		flag.IntVar(&Verbose, "verbose", 1, "Verbose level: \n"+
			"0: None\n"+
			"1: Typical errors\n"+
			"2: Connection flood\n"+
			"3: Timeout drops\n"+
			"4: Everything\n")
		help := flag.Bool("h", false, "Show help")
		flag.Parse()

		if *help {
			fmt.Println("GoPortForward")
			fmt.Println("Program by Publyo")
			fmt.Println("Source at https://github.com/Taulim/GoPortForward")
			fmt.Printf("Version %s\n", Version)
			flag.PrintDefaults()
			os.Exit(0)
		}

		if Verbose != 0 {
			fmt.Printf("Verbose mode on level %d\n", Verbose)
		}
	}

	// Read config file
	var conf Config
	{
		cfgBuffer, err := os.ReadFile(ConfigFileName)
		if err != nil {
			panic(fmt.Sprintf("Cannot read the config file. (IO Error) %s", err.Error()))
		}

		err = yaml.Unmarshal(cfgBuffer, &conf)
		if err != nil {
			panic(fmt.Sprintf("Cannot read the config file. (Parse Error) %s" + err.Error()))
		}

		ConnRules = conf.Rules

		MultiConn.Count = make([]int, len(ConnRules))
		if conf.Timeout <= 0 {
			ConnTimeout = 0
			logVerbose(1, "Timeout disabled")
		} else if conf.Timeout > 0 {
			ConnTimeout = time.Duration(conf.Timeout) * time.Second
			logVerbose(3, fmt.Sprintf("Timeout set to %d", ConnTimeout))
		}
	}

	// Start listeners
	for index, rule := range ConnRules {
		if rule.Listen == 0 {
			panic(fmt.Sprintf("Rule number %d does not have a valid port number", index+1))
		}

		if rule.Forward == "" {
			panic(fmt.Sprintf("Rule number %d does not have a forward address", index+1))
		}

		go func(i int, loopRule Rule) {
			log.Printf("Forwarding from port %d to %s\n", loopRule.Listen, loopRule.Forward)

			// Initialize the listener
			ln, err := net.Listen("tcp", fmt.Sprintf(":%d", int(loopRule.Listen)))
			if err != nil {
				panic(err)
			}

			for {
				// Accept new connections
				conn, err := ln.Accept()

				if err != nil {
					logVerbose(1, fmt.Sprintf("Error on accepting connection: %s", err.Error()))
					continue
				}

				go handleRequest(conn, i, loopRule)
			}
		}(index, rule)
	}

	// https://gobyexample.com/signals
	time.Sleep(1 * time.Second)
	sigs := make(chan os.Signal, 1)
	done := make(chan bool, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		// This will wait for a signal
		<-sigs
		done <- true
	}()
	log.Println("Ctrl + C to stop")
	<-done
	log.Println("Exiting")
}

// Handle incoming connections
// (conn / source) <-> (us / server) <-> (proxy / target)
func handleRequest(conn net.Conn, index int, r Rule) {
	// Check if the connection limit has been reached
	var hasReachedLimit = func(number int) bool {
		return (r.Simultaneous != 0) && (number >= (r.Simultaneous * 2))
	}

	var changeConnCount = func(number int) int {
		MultiConn.mu.Lock()
		defer MultiConn.mu.Unlock()

		if number != 0 {
			MultiConn.Count[index] += number
		}

		return MultiConn.Count[index]
	}

	var connCount = changeConnCount(0)
	var reachedLimit = hasReachedLimit(connCount)

	if reachedLimit {
		logVerbose(2, fmt.Sprintf("Connection limit is reached for %d Connection count was %d", r.Listen, (connCount/2)))
		conn.Close()
		return
	}

	// Open a connection to remote host
	proxy, err := net.Dial("tcp", r.Forward)
	if err != nil {
		logVerbose(1, fmt.Sprintf("Error on dialing remote host: %s", err.Error()))
		conn.Close()
		return
	}

	// Increase the connection count
	changeConnCount(2) // (conn -> us) and (us -> proxy)

	logVerbose(4, fmt.Sprintf("Accepted connection from %s", conn.RemoteAddr()))

	var removeConnCount = func() {
		changeConnCount(-1)
	}

	// client -> server
	go copyIO(conn, proxy, index, removeConnCount)

	// server -> client
	go copyIO(proxy, conn, index, removeConnCount)
}

// Copies the src to dest
// Index is the rule index
func copyIO(src, dest net.Conn, index int, closeCb func()) {
	defer src.Close()
	defer dest.Close()

	var err error
	if ConnTimeout > 0 {
		_, err = copyBuffer(dest, src)
	} else {
		_, err = io.Copy(dest, src)
	}

	if err != nil {
		if strings.Contains(err.Error(), "i/o timeout") {
			logVerbose(3, fmt.Sprintf("Connection timed out from %s to %s", src.RemoteAddr(), dest.RemoteAddr()))
		} else if strings.HasPrefix(err.Error(), "cannot set timeout for") {
			if strings.HasSuffix(err.Error(), "use of closed network connection") {
				logVerbose(4, err.Error())
			} else {
				logVerbose(1, err.Error())
			}
		} else {
			logVerbose(4, fmt.Sprintf("Error on copyBuffer: %s", err.Error()))
		}
	}

	closeCb()
	logVerbose(4, fmt.Sprintf("Closed connection from %s", src.RemoteAddr()))
}

func copyBuffer(dst, src net.Conn) (written int64, err error) {
	// Build a 32kb buffer
	buf := make([]byte, 32768)
	for {
		err = src.SetDeadline(time.Now().Add(ConnTimeout))
		if err != nil {
			err = errors.New("cannot set timeout for src:" + err.Error())
			break
		}
		nr, er := src.Read(buf)
		if nr > 0 {
			err = dst.SetDeadline(time.Now().Add(ConnTimeout))
			if err != nil {
				err = errors.New("cannot set timeout for dest: " + err.Error())
				break
			}
			nw, ew := dst.Write(buf[0:nr])
			if nw > 0 {
				written += int64(nw)
			}
			if ew != nil {
				err = ew
				break
			}
			if nr != nw {
				err = io.ErrShortWrite
				break
			}
		}
		if er != nil {
			if er != io.EOF {
				err = er
			}
			break
		}
	}
	return written, err
}

func logVerbose(level int, msg string) {
	if Verbose >= level {
		log.Println(msg)
	}
}
