package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type compileOptions struct {
	Package string
	Output  string
}

func (c *compileOptions) String() string {
	return fmt.Sprintf("-p: %s, -o: %s", c.Package, c.Output)
}

func main() {
	//1.创建本地文件记录编译过程中执行的命令，相当于记日志。
	writeArgs()
	//2.解析编译命令并转化为 compileOptions 结构体。
	args := os.Args[1:]
	option := parseCompileOption(args)
	//3.如果编译指令不为空，则进行增强方法插入。
	if option != nil && option.Package != "" && option.Output != "" {
		newArgs, err := instrument(args, option)
		if err != nil {
			log.Fatal(err)
		}
		args = newArgs
	}
	//4.执行增强后的编译指令。
	executeCommand(args, option)
}

// writeArgs 把编译过程中增强方法执行的指令记录在本地文件。
func writeArgs() {
	file, err := os.OpenFile("./go-agent-instrument.txt", os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0666)
	if err != nil {
		os.Exit(1)
	}
	defer file.Close()
	file.Write([]byte(fmt.Sprintf("%v\n", os.Args[1:])))
}

// parseCompileOption 把命令行编译指令解析成结构体 compileOptions ：
// go build -a -work -toolexec D:\Project\Golang\go-agent-instrumentation\cmd\cmd.exe .
func parseCompileOption(args []string) *compileOptions {
	if len(args) == 0 {
		return nil
	}

	cmd := filepath.Base(args[0])
	if ext := filepath.Ext(cmd); ext != "" {
		cmd = strings.TrimSuffix(cmd, ext)
	}
	if cmd != "compile" {
		return nil
	}

	opt := &compileOptions{}
	i := 1
	for i < len(args)-1 {
		if args[i][0] != '-' {
			i += 1
			continue
		}

		kv := strings.SplitN(args[i], "=", 2)
		var valRef *string
		if kv[0] == "-p" {
			valRef = &opt.Package
		} else if kv[0] == "-o" {
			valRef = &opt.Output
		} else {
			if len(kv) == 2 {
				i += 1
			} else if args[i+1] == "" || (len(args[i+1]) > 1 && args[i+1][0] != '-') {
				i += 2
			} else {
				i += 1
			}
			continue
		}

		if len(kv) == 2 {
			*valRef = kv[1]
			i += 1
		} else {
			*valRef = args[i+1]
			i += 2
		}
	}

	return opt
}

// executeCommand 执行解析后的编译指令。
func executeCommand(args []string, opt *compileOptions) error {
	path := args[0]
	args = args[1:]
	cmd := exec.Command(path, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
