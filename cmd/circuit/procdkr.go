// Copyright 2013 The Go Circuit Project
// Use of this source code is governed by the license for
// The Go Circuit Project, found in the LICENSE file.
//
// Authors:
//   2013 Petar Maymounkov <p@gocircuit.org>

package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"

	"github.com/gocircuit/circuit/client"
	"github.com/gocircuit/circuit/client/docker"
	"github.com/gocircuit/circuit/kit/iomisc"
	"github.com/pkg/errors"

	"github.com/urfave/cli"
)

// circuit mkproc /X1234/hola/charlie << EOF
// { … }
// EOF
// TODO: Proc element disappears if command misspelled and error condition not obvious.
func mkproc(x *cli.Context) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.Wrapf(r.(error), "error, likely due to missing server or misspelled anchor: %v", r)
		}
	}()
	c := dial(x)
	args := x.Args()
	if len(args) != 1 {
		return errors.New("mkproc needs an anchor argument")
	}
	w, _ := parseGlob(args[0])
	buf, _ := ioutil.ReadAll(os.Stdin)
	var cmd client.Cmd
	if err = json.Unmarshal(buf, &cmd); err != nil {
		return errors.Wrapf(err, "command json not parsing: %v", err)
	}
	if x.Bool("scrub") {
		cmd.Scrub = true
	}
	p, err := c.Walk(w).MakeProc(cmd)
	if err != nil {
		return errors.Wrapf(err, "mkproc error: %s", err)
	}
	ps := p.Peek()
	if ps.Exit != nil {
		return errors.Errorf("%v", ps.Exit)
	}
	return
}

func doRun(x *cli.Context, c *client.Client, cmd client.Cmd, path string, done chan bool) {

	w2, _ := parseGlob(path)
	a2 := c.Walk(w2)
	_runproc(x, c, a2, cmd, done)

}

func runproc(x *cli.Context) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.Wrapf(r.(error), "error, likely due to missing server or misspelled anchor: %v", r)
		}
	}()
	c := dial(x)
	args := x.Args()

	if len(args) != 1 && !x.Bool("all") {
		return errors.New("runproc needs an anchor argument or use the --all flag to to execute on every host in the circuit")
	}
	buf, _ := ioutil.ReadAll(os.Stdin)
	var cmd client.Cmd
	if err := json.Unmarshal(buf, &cmd); err != nil {
		return errors.Wrapf(err, "command json not parsing")
	}
	cmd.Scrub = true

	el := "/runproc/" + keygen(x)

	done := make(chan bool, 10)
	if x.Bool("all") {

		w, _ := parseGlob("/")

		anchor := c.Walk(w)

		procs := 0

		for _, a := range anchor.View() {

			procs++

			go func(x *cli.Context, cmd client.Cmd, a string, done chan bool) {

				doRun(x, c, cmd, a, done)

			}(x, cmd, a.Path()+el, done)

		}

		for ; procs > 0 ; procs--  {

			select {
			case <-done:
				continue
			}

		}

	} else {

		doRun(x, c, cmd, args[0]+el, done)
		<-done

	}

	return
}

func _runproc(x *cli.Context, c *client.Client, a client.Anchor, cmd client.Cmd, done chan bool) (err error) {

	p, err := a.MakeProc(cmd)
	if err != nil {
		return errors.Wrapf(err, "mkproc error")
	}

	stdin := p.Stdin()
	if err := stdin.Close(); err != nil {
		return errors.Wrapf(err, "error closing stdin")
	}

	if x.Bool("tag") {

		stdout := iomisc.PrefixReader(a.Addr() + " ", p.Stdout())
		stderr := iomisc.PrefixReader(a.Addr() + " ", p.Stderr())

		stdoutScanner := bufio.NewScanner(stdout)
		for stdoutScanner.Scan() {
			fmt.Println(stdoutScanner.Text())
		}

		stderrScanner := bufio.NewScanner(stderr)
		for stderrScanner.Scan() {
			fmt.Println(stderrScanner.Text())
		}

	} else {

		io.Copy(os.Stdout, p.Stdout())
		io.Copy(os.Stderr, p.Stderr())

	}
	p.Wait()
	done <- true

	return
}

func mkdkr(x *cli.Context) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.Wrapf(r.(error), "error, likely due to missing server or misspelled anchor: %v", r)
		}
	}()

	c := dial(x)
	args := x.Args()
	if len(args) != 1 {
		return errors.New("mkdkr needs an anchor argument")
	}
	w, _ := parseGlob(args[0])
	buf, _ := ioutil.ReadAll(os.Stdin)
	var run docker.Run
	if err = json.Unmarshal(buf, &run); err != nil {
		return errors.Wrapf(err, "command json not parsing: %v", err)
	}
	if x.Bool("scrub") {
		run.Scrub = true
	}
	if _, err = c.Walk(w).MakeDocker(run); err != nil {
		return errors.Wrapf(err, "mkdkr error: %s", err)
	}
	return
}

// circuit signal kill /X1234/hola/charlie
func sgnl(x *cli.Context) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.Wrapf(r.(error), "error, likely due to missing server or misspelled anchor: %v", r)
		}
	}()

	c := dial(x)
	args := x.Args()
	if len(args) != 2 {
		return errors.New("signal needs an anchor and a signal name arguments")
	}
	w, _ := parseGlob(args[1])
	u, ok := c.Walk(w).Get().(interface {
		Signal(string) error
	})
	if !ok {
		return errors.New("anchor is not a process or a docker container")
	}
	if err = u.Signal(args[0]); err != nil {
		return errors.Wrapf(err, "signal error: %v", err)
	}
	return
}

func wait(x *cli.Context) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.Wrapf(r.(error), "error, likely due to missing server or misspelled anchor: %v", r)
		}
	}()

	c := dial(x)
	args := x.Args()
	if len(args) != 1 {
		return errors.New("wait needs one anchor argument")
	}
	w, _ := parseGlob(args[0])

	var stat interface{}
	switch u := c.Walk(w).Get().(type) {
	case client.Proc:
		stat, err = u.Wait()
	case docker.Container:
		stat, err = u.Wait()
	default:
		return errors.New("anchor is not a process or a docker container")
	}
	if err != nil {
		return errors.Wrapf(err, "wait error: %v", err)
	}
	buf, _ := json.MarshalIndent(stat, "", "\t")
	fmt.Println(string(buf))
	return
}
