package nodeCheck

import (
	"errors"
	"time"

	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/pkg/api/v1/pod"
)

// checkNode checks the node's age to make sure it's not less than three minutes old. If so, sleep for one minute
// and check if kube proxy is ready and running on the node before running the check. This function requires the
// khcheck to expose the pod's node name information using environment variables.
func CheckNode(client *kubernetes.Clientset, nodeName string, now time.Time) {

	node, err := client.CoreV1().Nodes().Get(nodeName, v1.GetOptions{})
	if err != nil {
		log.Errorln("Failed to get node:", nodeName, err)
		return
	}

	nodeMinAge := time.Minute * 3
	sleepDuration := time.Minute
	nodeAge := now.Sub(node.CreationTimestamp.Time)
	// if the node the pod is on is less than 3 minutes old, wait 1 minute before running check.
	log.Infoln("Check running on node: ", node.Name, "with node age:", nodeAge)
	if nodeAge < nodeMinAge {
		log.Infoln("Node is than", nodeMinAge, "old. Sleeping for", sleepDuration)
		time.Sleep(sleepDuration)

		log.Infoln("Checking if kube-proxy is running and ready.")

		select {
		case err := <- waitForKubeProxyReady(client, node.Name):
			if err != nil {
				// Just log the error.
				log.Errorln(err)
			}
			log.Infoln("Kube proxy is ready. Proceeding to run check.")
		case <- time.After(time.Duration(time.Minute)):
			// Just log the error.
			// TO DO: figure out how to address this. Should the check to skip this run and pass up an error instead?
			// If kube-proxy isn't ready and running, there's definitely something wrong with the new node coming up.
			log.Errorln("Timed out checking if kube proxy is ready. Check node:", nodeName,
			"Check may or may not complete successfully.")
		}
		return
	}
	return
}

// waitForKubeProxyReady fetches the kube proxy pod every 5 seconds until it's ready and running.
func waitForKubeProxyReady(client *kubernetes.Clientset, nodeName string) chan error {

	kubeProxyName := "kube-proxy-" + nodeName
	log.Infoln("Waiting for kube-proxy pod to be ready:", kubeProxyName, "on node:", nodeName)
	doneChan := make(chan error, 1)

	for {
		kubeProxyReady, err := checkKubeProxyPod(client, kubeProxyName)
		if err != nil {
			log.Errorln("Error getting kube proxy pod:", err)
			doneChan <- err
			return doneChan
		}

		if kubeProxyReady {
			log.Infoln("Kube proxy: ", kubeProxyName, "is ready!")
			return doneChan
		}
		time.Sleep(time.Second * 5)
	}
}

// checkKubeProxyPod gets the kube proxy pod and checks if its ready and running.
func checkKubeProxyPod(client *kubernetes.Clientset, podName string) (bool, error) {

	var kubeProxyReady bool

	kubeProxyPod, err := client.CoreV1().Pods("kube-system").Get(podName, v1.GetOptions{})
	if err != nil {
		errorMessage := "Failed to get kube-proxy pod: " + podName + ". Error: " + err.Error()
		log.Errorln(errorMessage)
		return kubeProxyReady, errors.New(errorMessage)
	}

	if kubeProxyPod.Status.Phase == corev1.PodRunning && pod.IsPodReady(kubeProxyPod) {
		log.Infoln(kubeProxyPod.Name, "is in status running and ready.")
		kubeProxyReady = true
		return kubeProxyReady, nil
	}

	log.Infoln(kubeProxyPod.Name, "is in status:", kubeProxyPod.Status.Phase, "and ready: ",
		pod.IsPodReady(kubeProxyPod))
	return kubeProxyReady, nil
}
