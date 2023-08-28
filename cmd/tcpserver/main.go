package main

import (
	"flag"
	"log/slog"
	"os"
	"tcpserverv2/tcpserver"
)

func main() {
	var debugArg bool = false
	flag.BoolVar(&debugArg, "debug", true, "Debug mode, désactivé par défaut")
	flag.Parse()

	var displayLoggerSource bool = false // affiche le fichier et la ligne du fichier dans les logs
	// initialisation du système de log
	var logLevel slog.LevelVar
	if debugArg {
		displayLoggerSource = true
		logLevel.Set(slog.LevelDebug)
	}

	loggerHandler := slog.HandlerOptions{
		Level:     &logLevel,
		AddSource: displayLoggerSource,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr { // redéfinition des clés:
			if a.Key == slog.TimeKey {
				return slog.String("time", a.Value.Time().Format("2006-01-02 15:04:05")) // modifie la valeur du temps pour afficher l'heure comme sur les logs normaux
			}
			/*
				if a.Key == slog.LevelKey { // renomme la clé level
					a.Key = "level"
				}*/
			return a
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &loggerHandler))
	slog.SetDefault(logger) // redéfini logger en tant que logger pour slog

	//srv := tcpserver.TcpServer{Ip: "0.0.0.0", Port: "5555"}
	srv := tcpserver.NewTcpServer("0.0.0.0", "5555")
	srv.ListenAndServe()

}
