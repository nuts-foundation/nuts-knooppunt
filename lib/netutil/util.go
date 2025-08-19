package netutil

import "net"

// FreeTCPPort asks the kernel for a free open port that is ready to use.
// Taken from https://gist.github.com/sevkin/96bdae9274465b2d09191384f86ef39d
func FreeTCPPort() (port int, err error) {
	a, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		return 0, err
	}
	l, err := net.ListenTCP("tcp", a)
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}
