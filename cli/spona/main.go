package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
)

const version = "dev"

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("spona", flag.ContinueOnError)
	flags.SetOutput(stderr)

	showVersion := flags.Bool("version", false, "버전 출력")
	flags.Usage = func() {
		fmt.Fprintln(stderr, "사용법: spona [--version]")
		fmt.Fprintln(stderr)
		fmt.Fprintln(stderr, "에이전트 실행 환경을 프리셋으로 저장하고 실행합니다.")
		fmt.Fprintln(stderr)
		flags.PrintDefaults()
	}

	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("알 수 없는 인자: %s", flags.Arg(0))
	}
	if *showVersion {
		fmt.Fprintf(stdout, "spona %s\n", version)
		return nil
	}

	flags.Usage()
	return errors.New("명령을 지정해 주세요")
}
