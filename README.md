# gotcpserverv2
Version 2 du serveur tcp de chat en Go

La v1 est disponible [ici](https://github.com/loonaire/gotcpserver)

Cette version intègre beaucoup de modifications. La base du serveur se base sur l'exemple du livre Go Programming Language et utilise les channels. La fonction Read des sockets est également remplacée par un Scanner qui allège l'utilisation du CPU (sur linux, avec htop, on passe de 100% CPU sur la v1 à 0% sur la v2).
