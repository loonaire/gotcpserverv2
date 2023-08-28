package tcpserver

import (
	"fmt"
	"log"
	"log/slog"
	"net"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	TIMEOUT             = 3600 * time.Second
	SIZEOUTBUFFER       = 10
	SIZEINBUFFER        = 10
	MAXCONNECTEDCLIENTS = 3000
)

type TcpServer struct {
	Ip       string
	Port     string
	listener *net.TCPListener

	clients map[string]*client // clients connecté par nom

	entering          chan client
	leaving           chan *client
	broadcastMessages chan string
	shutdown          chan int
	stopListench      chan bool
	mu                sync.RWMutex
	wg                sync.WaitGroup
}

func NewTcpServer(ip string, port string) *TcpServer {
	return &TcpServer{Ip: ip, Port: port, clients: make(map[string]*client, MAXCONNECTEDCLIENTS), entering: make(chan client, 10), leaving: make(chan *client, 10), broadcastMessages: make(chan string, 10), mu: sync.RWMutex{}, shutdown: make(chan int, 1), stopListench: make(chan bool, 1)}
}

func (s *TcpServer) ListenAndServe() {
	addr, errAddr := net.ResolveTCPAddr("tcp", s.Ip+":"+s.Port)
	if errAddr != nil {
		slog.Error("Impossible de résoudre l'adresse ip ou le port")
		return
	}
	var errListen error
	s.listener, errListen = net.ListenTCP("tcp", addr)
	if errListen != nil {
		slog.Error(fmt.Sprintln("Impossible d'écouter sur l'adresse ", s.Ip, " et sur le port ", s.Port))
		return
	}

	log.Println("Serveur démarré sur l'ip:", s.Ip, " et sur le port:", s.Port)

	defer s.wg.Wait()

	s.wg.Add(1)
	go s.broadcaster()

stopListen:
	for {
		conn, err := s.listener.AcceptTCP()
		if err != nil {
			select {
			case <-s.stopListench:
				slog.Info("Arret de l'acceptation des connexions entrantes")
				break stopListen
			default:
				slog.Error(fmt.Sprintln("Erreur inconnue: ", err))
				continue
			}
		}
		s.wg.Add(1)
		go s.handleConn(conn)
	}
	s.stop()
	slog.Info("Serveur arrêté proprement")
}

func (s *TcpServer) stop() {
	for _, c := range s.clients {
		// déconnecte chaque client en forcant leurs déconnexion
		s.mu.Lock()
		delete(s.clients, c.name)
		s.mu.Unlock()
		c.forceDisconnect <- true
		c.disconnect()
	}
	close(s.broadcastMessages)
	close(s.leaving)

}

func (s *TcpServer) handleConn(conn *net.TCPConn) {
	defer s.wg.Done()
	defer func() {
		// fermeture de la connexion
		slog.Info(fmt.Sprintln("Déconnexion du client", conn.RemoteAddr()))
		err := conn.Close()
		if err != nil {
			slog.Error(fmt.Sprintln("Erreur lors de la fermeture de la connexion: ", err.Error()))
		}
	}()

	client := &client{
		conn:            conn,
		outch:           make(chan string),
		inch:            make(chan string),
		leave:           make(chan bool, 1),
		forceDisconnect: make(chan bool, 1),
	}

	go client.recv()
	go client.writer()

	client.outch <- "Input your name:"
	// Les clients qui ne répondent pas à leur nom dans le délai imparti seront déconnectés.
	select {
	case in, ok := <-client.inch:
		if !ok {
			client.disconnect()
			return
		}
		if s.checkNameAlreadyUsed(in) {
			// nom déja utilisé, on deconnecte le client
			slog.Warn(fmt.Sprintln("Erreur, nom " + in + " déja utilisé"))
			client.disconnect()
			return
		}
		slog.Info(fmt.Sprintln("Nouveau client connecté: ", in))
		client.name = in

	case <-time.After(TIMEOUT):
		return
	}
	s.broadcastMessages <- client.name + " has arrived"
	s.entering <- *client

	// boucle de reception des messages
stopConn:
	for {
		select {
		case in, ok := <-client.inch:
			if ok {
				// si la reception du message est valide, on traite le paquet
				handlePacket(s, client, in)
			} else {
				// si le canal inch est fermé le client est deconnecté
				s.leaving <- client
				break stopConn
			}
		case <-client.leave:
			// déconnexion "soft" du client
			s.leaving <- client
			break stopConn
		case <-client.forceDisconnect:
			// déconnexion suite à l'arret du serveur
			break stopConn
		case <-time.After(TIMEOUT):
			// déconnexion suite à un timeout
			s.leaving <- client
			break stopConn
		}
	}
}

func (s *TcpServer) broadcaster() {
	defer s.wg.Done()
	for {
		select {
		case msg := <-s.broadcastMessages:
			// Broadcast incoming message to all
			for _, cli := range s.clients {
				cli.outch <- msg
			}
		case cli := <-s.entering:
			// arrivée d'un nouveau client
			s.mu.Lock()
			s.clients[cli.name] = &cli
			//Informez les nouveaux clients de votre collection de clients actuelle.
			var onlines []string
			for _, c := range s.clients {
				onlines = append(onlines, c.name)
			}
			cli.outch <- fmt.Sprintf("%d clients: %s", len(s.clients), strings.Join(onlines, ", "))
			s.mu.Unlock()
		case cli := <-s.leaving:
			// déconnexion d'un client
			s.mu.Lock()
			delete(s.clients, cli.name)
			cli.disconnect()
			s.mu.Unlock()
			s.broadcastMessages <- cli.name + " has left"
		case <-s.shutdown:
			// arret du serveur
			slog.Info("Démarrage de l'arret du serveur...")
			s.stopListench <- true
			close(s.shutdown)
			err := s.listener.Close()
			if err != nil {
				slog.Error(fmt.Sprintln("Erreur lors de l'arrêt de l'écoute du serveur " + err.Error()))
			}
			return
		}
	}
}

func (s *TcpServer) sendTo(name string, message string) {
	s.clients[name].outch <- message
}

func (s *TcpServer) checkNameAlreadyUsed(name string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, isConnected := s.clients[name]; isConnected {
		// vérifie si le client veux utiliser un nom déja utlisé
		return true
	}
	return false
}

func handlePacket(srv *TcpServer, cli *client, message string) {
	// gère le paquet reçu, si le premier caractère n'est pas un /, il s'agit d'un message global a broadcaster
	if len(message) == 0 {
		return
	}
	if string(message[0]) != "/" {
		srv.broadcastMessages <- cli.name + ":" + message
	} else {
		// si le premier caractère est différent de /, il s'agit d'une commande
		slog.Info(fmt.Sprintln("Commande reçue: ", message))
		command := strings.Split(message[1:], " ")
		switch command[0] {
		case "help":
			// message d'aide:
			cli.outch <- "Liste des commandes:\n- help : affiche la liste des commandes"
		case "w":
			if srv.checkNameAlreadyUsed(command[1]) {
				cli.outch <- "To " + command[1] + " : " + strings.Join(command[2:], " ")
				srv.clients[command[1]].outch <- "from " + command[1] + " : " + strings.Join(command[2:], " ")
			}
		case "infos", "mem", "stats":
			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			cli.outch <- "Version de Go: " + runtime.Version() + " " + runtime.GOOS + " " + runtime.GOARCH + "\nNombre de CPU: " + strconv.Itoa(runtime.NumCPU()) + "\nNombre de goroutines: " + strconv.Itoa(runtime.NumGoroutine()) + "\nRam utilisée: " + strconv.FormatFloat(float64(m.Alloc)/1_000_000, 'f', 3, 32) + " Mo"
			cli.outch <- "Nombre de clients connectés: " + strconv.Itoa(len(srv.clients))
			cli.outch <- "Alloc = " + strconv.FormatFloat(float64(m.Alloc)/1024/1024, 'f', 2, 64) + "\nTotalAlloc = " + strconv.FormatFloat(float64(m.TotalAlloc/1024/1024), 'f', 2, 64) + "MiB"
			cli.outch <- "Sys = " + strconv.FormatFloat(float64(m.Sys)/1024/1024, 'f', 2, 64) + " MiB"
			cli.outch <- "NumGC = " + strconv.FormatUint(uint64(m.NumGC), 10)
		case "list":
			for k := range srv.clients {
				cli.outch <- k
			}
		case "shutdown":
			srv.shutdown <- 1
		case "quit", "leave":
			log.Println("Deconnexion du client " + cli.name)
			cli.leave <- true
		default:
			cli.outch <- "Commande inconnue, utilisez la commande /help pour afficher la liste des commandes disponibles"
		}
	}
}
