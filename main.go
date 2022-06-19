package main

import (
	"context"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/Riskified/k8s-controller-sidecars/controller"
	"github.com/Riskified/k8s-controller-sidecars/handler"
	log "github.com/sirupsen/logrus"
	api_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/workqueue"
)

var lvl log.Level

func CreateKubeClient() *kubernetes.Clientset {
	var config *rest.Config
	var err error
	// In cluster config
	if os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
		config, err = rest.InClusterConfig()
		if err != nil {
			log.Fatalf("Failed to load inCluster kube config\n%s", err.Error())
		}
	} else {
		// Local development mode
		kubeconfig := filepath.Join(os.Getenv("HOME"), ".kube", "config")
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			log.Fatalf("Failed to load kube config\n%s", err.Error())
		}
	}
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatalf("Failed create kube client\n%s", err.Error())
	}
	log.Debugf("Successfully constructed k8s client")
	return client

}
func initLog() {
	if os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
		lvl, _ = log.ParseLevel("info")
		log.SetFormatter(&log.TextFormatter{
			DisableColors: true,
			FullTimestamp: true},
		)
	} else {
		lvl, _ = log.ParseLevel("debug")
		log.SetFormatter(&log.TextFormatter{
			ForceColors:   true,
			FullTimestamp: true},
		)
	}
	log.SetLevel(lvl)

}

// main code path
func main() {
	initLog()
	// get the Kubernetes client for connectivity
	client := CreateKubeClient()

	// namespace := meta_v1.NamespaceDefault
	namespace := meta_v1.NamespaceAll

	// create the informer so that we can not only list resources
	// but also watch them for all pods in the default namespace
	informer := cache.NewSharedIndexInformer(
		// the ListWatch contains two different functions that our
		// informer requires: ListFunc to take care of listing and watching
		// the resources we want to handle
		&cache.ListWatch{
			ListFunc: func(options meta_v1.ListOptions) (runtime.Object, error) {
				// list all of the pods (core resource) in the deafult namespace
				return client.CoreV1().Pods(namespace).List(context.Background(), options)
			},
			WatchFunc: func(options meta_v1.ListOptions) (watch.Interface, error) {
				// watch all of the pods (core resource) in the default namespace
				return client.CoreV1().Pods(namespace).Watch(context.Background(), options)
			},
		},
		&api_v1.Pod{}, // the target type (Pod)
		0,             // no resync (period of 0)
		cache.Indexers{},
	)

	// create a new queue so that when the informer gets a resource that is either
	// a result of listing or watching, we can add an idenfitying key to the queue
	// so that it can be handled in the handler
	queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())

	// add event handlers to handle the three types of events for resources:
	//  - adding new resources
	//  - updating existing resources
	//  - deleting resources
	informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			// convert the resource object into a key (in this case
			// we are just doing it in the format of 'namespace/name')
			key, err := cache.MetaNamespaceKeyFunc(obj)
			log.Debugf("Add pod: %s", key)
			if err == nil {
				// add the key to the queue for the handler to get
				queue.Add(key)
				log.Debugf("Queue len: %d", queue.Len())
			}
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(newObj)
			log.Debugf("Update pod: %s", key)
			if err == nil {
				queue.Add(key)
				log.Debugf("Queue len: %d", queue.Len())
			}
		},
		DeleteFunc: func(obj interface{}) {
			// DeletionHandlingMetaNamsespaceKeyFunc is a helper function that allows
			// us to check the DeletedFinalStateUnknown existence in the event that
			// a resource was deleted but it is still contained in the index
			//
			// this then in turn calls MetaNamespaceKeyFunc
			key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
			log.Debugf("Delete pod: %s", key)
			if err == nil {
				queue.Add(key)
				log.Debugf("Queue len: %d", queue.Len())
			}
		},
	})

	// construct the Controller object which has all of the necessary components to
	// handle logging, connections, informing (listing and watching), the queue,
	// and the handler
	controller := controller.Controller{
		Logger:    log.NewEntry(log.New()),
		Clientset: client,
		Informer:  informer,
		Queue:     queue,
		Handler:   &handler.SidecarShutdownHandler{},
	}

	// use a channel to synchronize the finalization for a graceful shutdown
	stopCh := make(chan struct{})
	defer close(stopCh)

	// run the controller loop to process items
	go controller.Run(stopCh)

	// use a channel to handle OS signals to terminate and gracefully shut
	// down processing
	sigTerm := make(chan os.Signal, 1)
	signal.Notify(sigTerm, syscall.SIGTERM)
	signal.Notify(sigTerm, syscall.SIGINT)
	<-sigTerm

	log.Info("Shutting down....")
}
