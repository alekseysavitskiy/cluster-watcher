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
	_, err := k.Delete(context.Background(), key, &etcd.DeleteOptions{Dir: true, Recursive: true})
	etcdHandleErr(err)
	log.Printf("etcd rm key %s is done\n", key)
}
func etcdSetKey(k etcd.KeysAPI, key string) {
	_, err := k.Set(context.Background(), key, "", &etcd.SetOptions{Dir: true})
	etcdHandleErr(err)
}

func etcdSetValue(k etcd.KeysAPI, key string, value string) {
	_, err := k.Set(context.Background(), key, value, &etcd.SetOptions{Dir: false})
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
func etcdCleanupInfo(kapi etcd.KeysAPI, inspect types.ContainerJSON) {
	swarmNodeIde := inspect.Config.Labels["com.docker.swarm.node.id"]
	etcdRmKey(kapi, "/swarm/container_id/"+inspect.ID)
	etcdRmKey(kapi, "/swarm/node_id/"+swarmNodeIde+"/service_name/"+inspect.Config.Labels["com.docker.swarm.service.name"]+"/container_id/"+inspect.ID)
}

func etcdPopulateContainerInfo(kapi etcd.KeysAPI, inspect types.ContainerJSON) {
	swarmNodeIde := inspect.Config.Labels["com.docker.swarm.node.id"]
	etcdSetValue(kapi, "/swarm/container_id/"+inspect.ID+"/container_hostname", inspect.Config.Hostname+inspect.Config.Domainname)
	for name, v := range inspect.NetworkSettings.Networks {
		etcdSetValue(kapi, "/swarm/container_id/"+inspect.ID+"/network/"+name+"/container_ip", v.IPAddress)
	}
	etcdSetValue(kapi, "/swarm/container_id/"+inspect.ID+"/swarm_node_id", swarmNodeIde)
	//"com.docker.compose.service"
	etcdSetValue(kapi, "/swarm/container_id/"+inspect.ID+"/service_name", inspect.Config.Labels["com.docker.swarm.service.name"])

	etcdSetValue(kapi, "/swarm/node_id/"+swarmNodeIde+"/service_name/"+inspect.Config.Labels["com.docker.swarm.service.name"]+"/container_id", inspect.ID)

	//log changes
	etcdPrintRec(kapi, "/swarm/container_id/"+inspect.ID)
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

	containers, err := cli.ContainerList(context.Background(), types.ContainerListOptions{})
	if err != nil {
		panic(err)
	}
	for _, container := range containers {
		inspect, err := cli.ContainerInspect(context.Background(), container.ID)
		if err != nil {
			panic(err)
		}
		etcdPopulateContainerInfo(kapi, inspect)
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
				inspect, err := cli.ContainerInspect(context.Background(), event.ID)
				if err != nil {
					panic(err)
				}
				etcdCleanupInfo(kapi, inspect)

			case "start":
				log.Printf("docker event action  %s \t\t %s\n", event.Action, event.ID)
				inspect, err := cli.ContainerInspect(context.Background(), event.ID)
				if err != nil {
					panic(err)
				}
				etcdPopulateContainerInfo(kapi, inspect)
			}

		case err := <-errch:
			panic(err)
		}
	}
}
