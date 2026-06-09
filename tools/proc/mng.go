package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

const START_COMMAND string = "start"
const RESTART_COMMAND string = "restart"
const STOP_COMMAND string = "stop"
const CONFIG_FILE string = "grap3peer-local.yml"

var NODE_CMD []string = []string{"-verbose", "1", "-id", "local-node", "-node_type", "1", "-bootstrap_nodes", "/ip4/51.15.253.142/udp/33331/quic/p2p/12D3KooWHuVDGbd5wipxgcsJK2zNcQGw57gKAe99nK6e5QW1ZmEM,/ip4/163.172.133.51/udp/33331/quic/p2p/12D3KooWCgEchkDty35gFpTdEyAv8iUd8CVniw2XaJbUWvhQYxF2"}
var userDefinedCmdArgs string

type CommandArgs struct {
	Command      string
	Node_Cmd_Arg string
}

func ParseCmdArgs() (*CommandArgs, error) {
	conf := &CommandArgs{}

	flag.StringVar(&conf.Command, "command", RESTART_COMMAND, "Command to execute for node process: restart, start, stop")
	flag.StringVar(&conf.Node_Cmd_Arg, "node_cmd_args", "", "Command line args to start node. This parameter will override defaults which is: "+strings.Join(NODE_CMD, " "))

	flag.Parse()

	if conf.Command != START_COMMAND && conf.Command != RESTART_COMMAND && conf.Command != STOP_COMMAND {
		return nil, fmt.Errorf("unknown command to manage node process: %s, expected one of [start, stop, restart]", conf.Command)
	}
	userDefinedCmdArgs = conf.Node_Cmd_Arg

	return conf, nil
}

func main() {
	fmt.Println("Start Node Process Management Tool")
	args, err := ParseCmdArgs()
	if err != nil {
		log.Fatalf("Invalid args supplied: %s\n", err.Error())
	}
	fmt.Printf("Received command: %s\n", args.Command)
	if args.Command == STOP_COMMAND {
		stopNode()
	}
	if args.Command == START_COMMAND {
		startNode()
	}
	if args.Command == RESTART_COMMAND {
		if fileExist(pidPath()) {
			stopNode()
		}
		startNode()
	}
}

func startNode() {
	pidPath := pidPath()
	if fileExist(pidPath) {
		log.Fatalf("Node process is already started by pid: %s", readFile(pidPath))
	}
	cmd_args := NODE_CMD
	if userDefinedCmdArgs != "" {
		cmd_args = strings.Split(userDefinedCmdArgs, " ")
	} else {
		cmd_args = append(cmd_args, "-f", CONFIG_FILE)
	}
	fmt.Printf("Launch node with cmd args '%s'", strings.Join(cmd_args, " "))
	peerExec := "grap3peer"
	if runtime.GOOS == "windows" {
		peerExec = peerExec + ".exe"
	}
	cmd := exec.Command(filepath.Join(currentDir(), peerExec), cmd_args...)
	cmd.Dir = currentDir() // set working directory
	err := cmd.Start()
	if err != nil {
		log.Fatalf("Failed to start command: %s", err.Error())
	}
	fmt.Printf("Started node with pid=%d \n", cmd.Process.Pid)
	writeFile(pidPath, strconv.Itoa(cmd.Process.Pid))
}

func stopNode() {
	pid := readPid()
	if err := killProcessByPID(pid); err != nil {
		fmt.Printf("Unable to stop node by pid: %d\n", pid)
	}
	if err := os.Remove(pidPath()); err != nil {
		log.Printf("Unable to remove pid file by path %s: %s\n", pidPath(), err.Error())
	}
	fmt.Printf("Stopped node by pid=%d", pid)
}

func readPid() int {
	pidPath := filepath.Join(homeDir(), ".grap3", "peer-process.pid")
	if !fileExist(pidPath) {
		log.Fatalf("Node process isn't running, no pid by path: %s", pidPath)
	}
	pid, _ := strconv.Atoi(readFile(pidPath))
	return pid
}

func fileExist(path string) bool {
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		fmt.Printf("File '%s' does not exist.\n", path)
		return false
	} else if err != nil {
		log.Fatalf("Failed to check file existence: %v\n", err)
	} else {
		fmt.Printf("File '%s' exists.\n", path)
		return true
	}
	return false
}

func readFile(path string) string {
	content, err := ioutil.ReadFile(path)
	if err != nil {
		log.Fatalf("Failed to read file: %v", err)
	}

	// Convert the file content to a string
	fileContent := string(content)
	return fileContent
}

func writeFile(path string, content string) {
	os.MkdirAll(filepath.Dir(path), os.ModePerm)
	file, err := os.Create(path)
	if err != nil {
		log.Fatalf("Failed to create file: %v", err)
	}
	defer file.Close()

	// Write the content to the file
	_, err = file.WriteString(content)
	if err != nil {
		log.Fatalf("Failed to write to file: %v", err)
	}
}

func homeDir() string {
	hd, err := os.UserHomeDir()
	if err != nil {
		log.Fatalf("No homedir found: %s\n", err.Error())
	}
	return hd
}

func killProcessByPID(pid int) error {
	switch runtime.GOOS {
	case "windows":
		cmd := exec.Command("taskkill", "/F", "/PID", strconv.Itoa(pid))
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	case "linux", "darwin":
		return killProcessBySignal(pid, os.Interrupt)
	default:
		return fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}
}

func killProcessBySignal(pid int, signal os.Signal) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return process.Signal(signal)
}

func pidPath() string {
	hd := homeDir()
	pidPath := filepath.Join(hd, ".grap3", "peer-process.pid")
	return pidPath
}

func currentDir() string {
	exePath, err := os.Executable()
	if err != nil {
		log.Fatalf("Failed to get executable path: %v\n", err)
	}

	// Get the directory of the executable
	dir := filepath.Dir(exePath)

	fmt.Printf("Current binary directory: %s\n", dir)
	return dir
}

func safePath(path string) string {
	return "\"" + path + "\""
}
