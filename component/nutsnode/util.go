package nutsnode

import "net"

// freeTCPPort asks the kernel for a free open port that is ready to use.
// Taken from https://gist.github.com/sevkin/96bdae9274465b2d09191384f86ef39d
func freeTCPPort() (port int) {
	if a, err := net.ResolveTCPAddr("tcp", "localhost:0"); err == nil {
		var l *net.TCPListener
		if l, err = net.ListenTCP("tcp", a); err == nil {
			defer l.Close()
			return l.Addr().(*net.TCPAddr).Port
		} else {
			panic(err)
		}
	} else {
		panic(err)
	}
}
