package main

import (
	"fmt"
	. "github.com/mailgun/vulcand/backend"
	"github.com/wsxiaoys/terminal/color"
	"strings"
)

const (
	vLine   = "\u2502"
	rCross  = "\u251C"
	lCorner = "\u2514"
)

type Tree interface {
	Self() string
	Children() []Tree
}

func printTree(root Tree, depth int, last bool, offset string) {
	// Print self
	tprint(fmt.Sprintf("%s%s%s", offset, getConnector(depth, last), root.Self()))

	// No children, we are done
	children := root.Children()
	if len(children) == 0 {
		return
	}

	// We have children, print connector offset
	tprint(getOffset(offset, last))
	// Compute child offset, in case if we are not the last child
	// add vertical line | to connect our parent to the last child
	childOffset := getChildOffset(offset, last)

	for i, c := range children {
		printTree(c, depth+1, i == len(children)-1, childOffset)
		if i != len(children)-1 {
			tprint(fmt.Sprintf("%s|", childOffset))
		}
	}
}

func getConnector(depth int, last bool) string {
	if depth == 0 {
		return ""
	}
	if last {
		return lCorner
	}
	return rCross
}

func getChildOffset(offset string, last bool) string {
	if last {
		return fmt.Sprintf("%s  ", offset)
	}
	// in case if we are not the last child
	// add vertical line | to connect our parent to the last child
	return fmt.Sprintf("%s| ", offset)
}

func getOffset(offset string, last bool) string {
	if last {
		return fmt.Sprintf("%s  |", offset)
	}
	return fmt.Sprintf("%s| |", offset)
}

type VulcanTree struct {
	root interface{}
}

func (vt *VulcanTree) Self() string {
	switch (vt.root).(type) {
	case []*Host:
		return "[hosts]"
	case []*Upstream:
		return "[upstreams]"
	case []*MiddlewareInstance:
		return "[middlewares]"
	}
	return formatInstance(vt.root)
}

func (vt *VulcanTree) Children() []Tree {
	switch r := (vt.root).(type) {
	case []*Host:
		return hostsToTrees(r)
	case []*Upstream:
		return upstreamsToTrees(r)
	case []*MiddlewareInstance:
		return middlewaresToTrees(r)
	case *Host:
		return locationsToTrees(r.Locations)
	case *Upstream:
		return endpointsToTrees(r.Endpoints)
	case *Location:
		children := []Tree{}
		if len(r.Middlewares) > 0 {
			children = append(children, &VulcanTree{root: r.Middlewares})
		}
		return append(children, upstreamsToTrees([]*Upstream{r.Upstream})...)
	}
	return nil
}

func hostsToTrees(in []*Host) []Tree {
	out := make([]Tree, len(in))
	for i, _ := range out {
		out[i] = &VulcanTree{root: in[i]}
	}
	return out
}

func locationsToTrees(in []*Location) []Tree {
	out := make([]Tree, len(in))
	for i, _ := range out {
		out[i] = &VulcanTree{root: in[i]}
	}
	return out
}

func upstreamsToTrees(in []*Upstream) []Tree {
	out := make([]Tree, len(in))
	for i, _ := range out {
		out[i] = &VulcanTree{root: in[i]}
	}
	return out
}

func endpointsToTrees(in []*Endpoint) []Tree {
	out := make([]Tree, len(in))
	for i, _ := range out {
		out[i] = &VulcanTree{root: in[i]}
	}
	return out
}

func middlewaresToTrees(in []*MiddlewareInstance) []Tree {
	out := make([]Tree, len(in))
	for i, _ := range out {
		out[i] = &VulcanTree{root: in[i]}
	}
	return out
}

func printResult(format string, in interface{}, err error) {
	if err != nil {
		printError(err)
	} else {
		printOk(format, formatInstance(in))
	}
}

func printStatus(in interface{}, err error) {
	if err != nil {
		printError(err)
	} else {
		printOk("%s", in)
	}
}

func printError(err error) {
	color.Print(fmt.Sprintf("@rERROR: %s\n", err))
}

func printOk(message string, params ...interface{}) {
	color.Print(fmt.Sprintf("@gOK: %s\n", fmt.Sprintf(message, params...)))
}

func printInfo(message string, params ...interface{}) {
	color.Print("INFO: @w%s\n", fmt.Sprintf(message, params...))
}

func printHosts(hosts []*Host) {
	tprint("")
	printTree(&VulcanTree{root: hosts}, 0, true, "")
}

func printUpstreams(upstreams []*Upstream) {
	tprint("")
	printTree(&VulcanTree{root: upstreams}, 0, true, "")
}

func tprint(out string, params ...interface{}) {
	s := fmt.Sprintf(out, params...)
	s = strings.Replace(s, "+-", rCross, -1)
	s = strings.Replace(s, "|", vLine, -1)
	fmt.Println(s)
}

func formatInstance(in interface{}) string {
	switch r := in.(type) {
	case *Host:
		return fmt.Sprintf("host[name=%s]", r.Name)
	case *Upstream:
		return fmt.Sprintf("upstream[id=%s]", r.Id)
	case *Endpoint:
		return fmt.Sprintf("endpoint[id=%s, url=%s]", r.Id, r.Url)
	case *Location:
		return fmt.Sprintf("location[id=%s, path=%s]", r.Id, r.Path)
	case *MiddlewareInstance:
		return fmt.Sprintf("%s[id=%s, %s]", r.Type, r.Id, r.Middleware)
	}
	return fmt.Sprintf("%s", in)
}
