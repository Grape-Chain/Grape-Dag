# How to enable P2P across multiple networks

## Launching bootstrap nodes
The first bootstrap nodes can be launched as follows:
```bash
./lunapeer -id <ID> -rendezvous <pubsub rendezvous name> -port <port> -bootstrap
...
[Private] /ip4/198.51.100.10/udp/33331/quic/p2p/QmS1L76o6q2CVG2AAtRC2PgSS4jznQce7k5ZtZT2LMbq89
```

The second bootstrap node must reference the first bootstrap node as follows:
```bash
./lunapeer -id <ID> -rendezvous <pubsub rendezvous name> -port <port> -bootstrap -bootstrap_nodes [Private] /ip4/198.51.100.10/udp/33331/quic/p2p/QmS1L76o6q2CVG2AAtRC2PgSS4jznQce7k5ZtZT2LMbq89
...
[Private] /ip4/198.51.100.20/udp/33331/quic/p2p/QmV9si1eabw3sdHHnHJZKgtJVQiv7T5dE57Q1Q3syZzzwj
```

You can choose between the tcp and udp protocols to communicate over:
```bash
2023-04-09T19:00:40.994Z	INFO	p2p-main	utils/colorize.go:30	[Private] /ip4/198.51.100.20/udp/33331/quic/p2p/QmV9si1eabw3sdHHnHJZKgtJVQiv7T5dE57Q1Q3syZzzwj

Host ID: /ip4/198.51.100.20/udp/33331/quic/p2p/QmV9si1eabw3sdHHnHJZKgtJVQiv7T5dE57Q1Q3syZzzwj

2023-04-09T19:00:40.995Z	INFO	p2p-main	utils/colorize.go:30	[Private] /ip4/198.51.100.20/tcp/33331/p2p/QmV9si1eabw3sdHHnHJZKgtJVQiv7T5dE57Q1Q3syZzzwj

Host ID: /ip4/198.51.100.20/tcp/33331/p2p/QmV9si1eabw3sdHHnHJZKgtJVQiv7T5dE57Q1Q3syZzzwj
```

## Launch a leader node
```bash
./lunapeer -id <ID> -rendezvous <pubsub rendezvous name> -port <port> -leader -bootstrap_nodes /ip4/198.51.100.10/udp/33331/quic/p2p/QmS1L76o6q2CVG2AAtRC2PgSS4jznQce7k5ZtZT2LMbq89,/ip4/198.51.100.20/udp/33331/quic/p2p/QmV9si1eabw3sdHHnHJZKgtJVQiv7T5dE57Q1Q3syZzzwj
```
Please note that must specify both bootstrap node connection strings. If the leader node runs on a network reachable on the network on which the bootstrap nodes run, you can use the IP addresses reported by the bootstrap nodes. Otherwise, these IP addresses must be replaced with reachable addresses.

## Launch a full node
```
./lunapeer -id <ID> -rendezvous <pubsub rendezvous name> -port <port> -bootstrap_nodes /ip4/198.51.100.10/udp/33331/quic/p2p/QmS1L76o6q2CVG2AAtRC2PgSS4jznQce7k5ZtZT2LMbq89,/ip4/198.51.100.20/udp/33331/quic/p2p/QmV9si1eabw3sdHHnHJZKgtJVQiv7T5dE57Q1Q3syZzzwj
```
After the node is launched, please wait for the node to discover other peers on the network, join to pubsubs and establish connections with the existing nodes.

## Launching a full node at home
You may want to run a lunaone node on your home network. Typically, you do not need to do anything special to connect, however, at times, you may need to set up port forwarding or enable UPnP to allow the node to configure itself for better connectivity. Double-NAT scenario may also be possible, if you set up port forwarding rules on all the NAT devices except the device immediately connected to the node. On this device you may want to turn on UPnP or manually set up the port forwarding rules.

When you want to enable the node's UPnP functionality to configure the port mappings, enable UPnP on your NAT device (router), and launch your node with the option -home
```bash
./lunapeer -id ..... <all usual options> -home ...
```