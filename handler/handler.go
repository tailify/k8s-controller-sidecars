package handler

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/avast/retry-go"

	log "github.com/sirupsen/logrus"
	core_v1 "k8s.io/api/core/v1"

	"github.com/Riskified/k8s-controller-sidecars/lib"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	set "github.com/deckarep/golang-set"
)

// Handler interface contains the methods that are required
type Handler interface {
	Init() error
	ObjectCreated(obj interface{})
	ObjectDeleted(obj interface{})
	ObjectUpdated(objOld, objNew interface{})
}

// SidecarShutdownHandler is a sample implementation of Handler
type SidecarShutdownHandler struct{}

// Init handles any handler initialization
func (t *SidecarShutdownHandler) Init() error {
	log.Info("SidecarShutdownHandler.Init")
	return nil
}

// Send a shutdown signal to all containers in the Pod
func sendShutdownSignal(pod *core_v1.Pod, containers set.Set) {
	log.Infof("terminating pod %s , with cointainers %s", pod.Name, containers)

	// Multiple arguments must be provided as separate "command" parameters
	// The first one is added automatically.
	// Todo: Update requestFromConfig to handle this better
	command := "sh&command=-c&command=kill+-s+TERM+1" // "kill -s TERM 1"
	// creates the connection
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

	// Create a round tripper with all necessary kubernetes security details
	wrappedRoundTripper, err := lib.RoundTripperFromConfig(config)

	if err != nil {
		log.Fatalln(err)
	}

	for _, c := range containers.ToSlice() {
		// Create a request out of config and the query parameters
		req, err := lib.RequestFromConfig(config, pod.Name, c.(string), pod.Namespace, command)
		if err != nil {
			log.Infoln(err)
		}

		err = retry.Do(
			func() error {
				// Send the request and let the callback do its work
				_, err = wrappedRoundTripper.RoundTrip(req)
				if err != nil {
					return err
				}
				return nil
			},
			retry.Delay(3*time.Second),
			retry.Attempts(5),
		)
		if err != nil {
			log.Errorln(err)
		}
	}
}

// ObjectCreated is called when an object is created
func (t *SidecarShutdownHandler) ObjectCreated(obj interface{}) {
	log.Debug("SidecarShutdownHandler.ObjectCreated")
	// assert the type to a Pod object to pull out relevant data
	pod := obj.(*core_v1.Pod)

	mainProcString, exists := pod.Annotations["riskified.com/main_sidecars"]
	sidecarsString, sidecarsAnnotationExists := pod.Annotations["riskified.com/sidecars"]

	if exists {
		log.Debugf("ResourceTrackable: true")
		log.Infof("pod: %s ; namespace: %s ; mainProc: %s ; node: %s", pod.Name, pod.Namespace, mainProcString, pod.Spec.NodeName)
	} else if sidecarsAnnotationExists {
		log.Debugf("sidecar ResourceTrackable: true")
		log.Infof("pod: %s ; namespace: %s ; sidecars: %s ; node: %s", pod.Name, pod.Namespace, sidecarsString, pod.Spec.NodeName)
	} else {
		return
	}

	mainProc := set.NewSet()
	sidecars := set.NewSet()

	for _, s := range strings.Split(sidecarsString, ",") {
		sidecars.Add(s)
	}

	for _, s := range strings.Split(mainProcString, ",") {
		mainProc.Add(s)
	}

	allContainers := set.NewSet()
	runningContainers := set.NewSet()
	completedContainers := set.NewSet()

	for _, containerStatus := range pod.Status.ContainerStatuses {
		allContainers.Add(containerStatus.Name)

		if containerStatus.Ready {
			runningContainers.Add(containerStatus.Name)
		} else {
			terminated := containerStatus.State.Terminated
			if terminated != nil && (terminated.Reason == "Completed" || terminated.Reason == "Error") {
				completedContainers.Add(containerStatus.Name)
			}
		}
	}
	log.Debugf("pod       : %s", pod.Name)
	log.Debugf("all       : %s", allContainers)
	log.Debugf("running   : %s", runningContainers)
	log.Debugf("completed : %s", completedContainers)
	log.Debugf("main  	  : %s", mainProc)

	// If we have accounted for all of the containers, and the sidecar containers are the only
	// ones still running, issue them each a shutdown command
	if runningContainers.Union(completedContainers).Equal(allContainers) {
		log.Debugf("We have all the containers")
		log.Debugf("main: %s, completed: %s, chech: %t", mainProc, completedContainers, completedContainers.Contains(mainProc.ToSlice()...))
		if completedContainers.Contains(mainProc.ToSlice()...) && len(runningContainers.ToSlice()) > 0 {
			log.Infof("Sending shutdown signal to containers %s in pod %s", runningContainers, pod.Name)
			sendShutdownSignal(pod, runningContainers)
		} else if runningContainers.Equal(sidecars) && len(runningContainers.ToSlice()) > 0 {
			log.Infof("Sending shutdown signal to containers %s in pod %s", runningContainers, pod.Name)
			sendShutdownSignal(pod, runningContainers)
		}
	}
}

// ObjectDeleted is called when an object is deleted
func (t *SidecarShutdownHandler) ObjectDeleted(obj interface{}) {
	log.Debug("SidecarShutdownHandler.ObjectDeleted")
}

// ObjectUpdated is called when an object is updated.
// Note that the controller in this repo will never call this function properly.
// It uses only ObjectCreated
func (t *SidecarShutdownHandler) ObjectUpdated(objOld, objNew interface{}) {
	log.Debug("SidecarShutdownHandler.ObjectUpdated")
}
