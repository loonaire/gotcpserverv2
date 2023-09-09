package tcpserver

import (
	"bufio"
	"errors"
	"fmt"
	"log/slog"
	"net"
)

const (
	MESSAGESUFFIX = "\n"
)

type client struct {
	conn            *net.TCPConn
	outch           chan string
	inch            chan string
	leave           chan bool
	forceDisconnect chan bool
	name            string
}

//Reception à l'aide d'un scanner:
func (c *client) recv() {
	input := bufio.NewScanner(c.conn)
	for input.Scan() {
		slog.Debug(fmt.Sprintf("%s - << %q\n", c.conn.RemoteAddr(), input.Text()))
		if len(input.Text()) > 0 {
			c.inch <- input.Text()
		}
	}
	if err := input.Err(); err != nil {
		// erreur sur le scan de la saisie
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			slog.Error(fmt.Sprintln("Erreur sur le buffer de reception d'un client ", err))
		}
	}
	close(c.inch)
}

/*
// Reception d'un paquet en utilisant un Reader:
func (c *client) recv() {
	// test reader:
	reader := bufio.NewReader(c.conn)
	for {
		msg, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			} else {
				var netErr net.Error
				if errors.As(err, &netErr) && netErr.Timeout() {
					log.Println(err)
				}
				return
			}
		}
		if len(msg) > 0 {
			msg := strings.TrimRight(msg, "\n")
			slog.Debug(fmt.Sprintf("%s - << %q", c.conn.RemoteAddr(), msg))
			c.inch <- string(msg)
		}
	}
	close(c.inch)
}
*/

func (c *client) writer() {
	for msg := range c.outch {
		msg += MESSAGESUFFIX
		_, err := fmt.Fprint(c.conn, msg)
		if err != nil {
			slog.Error(fmt.Sprintln("Erreur lors de l'envoi d'un message: ", err.Error()))
		}
		slog.Debug(fmt.Sprintf("%s - >> %q\n", c.conn.RemoteAddr(), msg))
	}
}

func (c *client) disconnect() {
	close(c.outch)
	close(c.leave)
	close(c.forceDisconnect)
}
