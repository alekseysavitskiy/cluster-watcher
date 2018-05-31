package main

import (
	"context"
	"log"
	"os"
	"time"

	etcd "github.com/coreos/etcd/client"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	docker "github.com/docker/docker/client"
)

func etcdRmKey(k etcd.KeysAPI, key string) {
	resp, err := k.Delete(context.Background(), key, &etcd.DeleteOptions{Dir: true, Recursive: true})
	log.Printf("Set is done. Metadata is %q\n", resp)
	etcdHandleErr(err)
}
func etcdSetKey(k etcd.KeysAPI, key string) {
	resp, err := k.Set(context.Background(), key, "", &etcd.SetOptions{Dir: true})
	log.Printf("Set is done. Metadata is %q\n", resp)
	etcdHandleErr(err)
}

func etcdSetValue(k etcd.KeysAPI, key string, value string) {
	_, err := k.Set(context.Background(), key, value, &etcd.SetOptions{Dir: false})
	//log.Printf("Set is done. Metadata is %q\n", resp)
	etcdHandleErr(err)
}

func etcdPrintRec(k etcd.KeysAPI, key string) {
	resp, err := k.Get(context.Background(), key, &etcd.GetOptions{Recursive: true})
	for _, node := range resp.Node.Nodes {
		if node.Dir {
			etcdPrintRec(k, node.Key)
		} else {
			log.Printf("%s %s \n", node.Key, node.Value)
		}
	}
	etcdHandleErr(err)
}
func etcdHandleErr(err error) {
	if err != nil {
		if err == context.Canceled {
			// ctx is canceled by another routine
			log.Printf("ctx is canceled by another routine\n")
		} else if err == context.DeadlineExceeded {
			// ctx is attached with a deadline and it exceeded
			log.Printf("ctx is attached with a deadline and it exceeded\n")
		} else if cerr, ok := err.(*etcd.ClusterError); ok {
			// process (cerr.Errors)
			log.Printf("%s \n", cerr.Errors)
			panic(err)
		} else {
			// bad cluster endpoints, which are not etcd servers
			log.Printf("bad cluster endpoints, which are not etcd servers\n")
			panic(err)
		}
	}
}

func etcdPopulateContainerInfo(kapi etcd.KeysAPI, inspect types.ContainerJSON, info types.Info) {
	etcdSetValue(kapi, "/swarm/container_id/"+inspect.ID+"/container_hostname", inspect.Config.Hostname+inspect.Config.Domainname)
	etcdSetValue(kapi, "/swarm/container_id/"+inspect.ID+"/container_ip", inspect.NetworkSettings.IPAddress)
	etcdSetValue(kapi, "/swarm/container_id/"+inspect.ID+"/swarm_node_id", info.Swarm.NodeID)
	//"com.docker.compose.service"
	etcdSetValue(kapi, "/swarm/container_id/"+inspect.ID+"/service_name", inspect.Config.Labels["com.docker.swarm.service.name"])
}

func main() {
	arg := os.Args[1]
	etcdURL := "http://127.0.0.1:2379"
	if len(arg) != 0 {
		etcdURL = arg
	}
	cfg := etcd.Config{
		Endpoints: []string{etcdURL},
		Transport: etcd.DefaultTransport,
		// set timeout per request to fail fast when the target endpoint is unavailable
		HeaderTimeoutPerRequest: time.Second,
	}
	c, err := etcd.New(cfg)
	if err != nil {
		log.Fatal(err)
	}

	kapi := etcd.NewKeysAPI(c)
	//etcdRmKey(kapi, "/swarm")

	// go client
	cli, err := docker.NewEnvClient()
	if err != nil {
		panic(err)
	}

	info, err := cli.Info(context.Background())
	if err != nil {
		panic(err)
	}

	containers, err := cli.ContainerList(context.Background(), types.ContainerListOptions{})
	if err != nil {
		panic(err)
	}
	for _, container := range containers {
		inspect, err := cli.ContainerInspect(context.Background(), container.ID)
		if err != nil {
			panic(err)
		}
		etcdPopulateContainerInfo(kapi, inspect, info)
	}

	etcdPrintRec(kapi, "/swarm")

	filters := filters.NewArgs()
	filters.Add("type", "container")
	eventch, errch := cli.Events(context.Background(), types.EventsOptions{Filters: filters})

	for {
		select {
		case event := <-eventch:
			//log.Printf("%s \t\t %s\n", event.Action, event)
			switch event.Status {
			case "die":
				log.Printf("docker event action %s \t\t %s\n", event.Action, event.ID)
				etcdRmKey(kapi, "/swarm/container_id/"+event.ID)
			case "start":
				log.Printf("docker event action  %s \t\t %s\n", event.Action, event.ID)
				inspect, err := cli.ContainerInspect(context.Background(), event.ID)
				if err != nil {
					panic(err)
				}
				etcdPopulateContainerInfo(kapi, inspect, info)
			}

		case err := <-errch:
			panic(err)
		}
	}
}
