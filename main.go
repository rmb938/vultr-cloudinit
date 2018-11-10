package main

import (
	"flag"
	"net"
	"os"
	"time"

	"context"

	"net/http"

	"io/ioutil"

	"encoding/json"

	"fmt"

	"strings"

	"path"

	"os/exec"

	"github.com/sirupsen/logrus"
	"golang.org/x/net/context/ctxhttp"
	"gopkg.in/yaml.v2"
)

type VultrInterfaces struct {
	Mac         string `json:"mac"`
	NetworkType string `json:"network-type"`
	NetworkID   string `json:"networkid"`
	IPv4        struct {
		Address string `json:"address"`
		Gateway string `json:"gateway"`
		Netmask string `json:"netmask"`
	} `json:"ipv4"`
}

type VultrMetadata struct {
	Hostname   string            `json:"hostname"`
	InstanceID string            `json:"instanceid"`
	Interfaces []VultrInterfaces `json:"interfaces"`
	PublicKeys string            `json:"public-keys"`
	Region     struct {
		RegionCode string `json:"regioncode"`
	} `json:"region"`
}

type NoCloudMetadata struct {
	AmiID            string   `json:"ami-id"`
	InstanceID       string   `json:"instance-id"`
	Region           string   `json:"region"`
	AvailabilityZone string   `json:"availability-zone"`
	Tags             []string `json:"tags"`
	PublicKeys       []string `json:"public-keys"`
	Hostname         string   `json:"hostname"`
	LocalHostname    string   `json:"local-hostname"`
}

type NoCloudNetworkSubnet struct {
	Type    string `yaml:"type"`
	Address string `yaml:"address"`
	Netmask string `yaml:"netmask"`
}

type NoCloudNetworkInterface struct {
	Type       string                 `yaml:"type"`
	Name       string                 `yaml:"name"`
	MacAddress string                 `yaml:"mac_address"`
	Subnets    []NoCloudNetworkSubnet `yaml:"subnets"`
	MTU        int                    `yaml:"mtu"`
}

type NoCloudNetworkConfig struct {
	Version int                       `yaml:"version"`
	Config  []NoCloudNetworkInterface `yaml:"config"`
}

func timeoutContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 120*time.Second)
}

func main() {
	outputDirectory := flag.String("o", "/var/lib/cloud/seed/nocloud/", "The output directory for NoCloud files")

	flag.Parse()

	if len(*outputDirectory) == 0 {
		logrus.Fatalf("The output directory must be given")
	}

	if f, err := os.Stat(*outputDirectory); !os.IsNotExist(err) {
		if !f.IsDir() {
			logrus.Fatalf("Output directory %s is not a directory", *outputDirectory)
		}
	} else {
		logrus.Fatalf("Error checking output directory: %s", err.Error())
	}

	logrus.Info("Starting DHCP Client on eth0")
	dhclient := exec.Command("/usr/sbin/dhclient", "-1", "-v", "-d", "eth0")
	go func() {
		err := dhclient.Run()
		if err != nil {
			if _, ok := err.(*exec.ExitError); !ok {
				logrus.Fatalf("Error running dhclient: %s", err.Error())
			}
		}
	}()
	defer func() {
		if dhclient.Process != nil {
			dhclient.Process.Kill()
		}
	}()

	logrus.Info("Waiting for eth0 to get an IP address")
	iface, err := net.InterfaceByName("eth0")

	if err != nil {
		logrus.Fatalf("Error getting eth0: %s", err.Error())
	}

	var eth0IP *net.IP

	for eth0IP == nil {
		addrs, err := iface.Addrs()
		if err != nil {
			logrus.Fatalf("Error getting eth0 addresses: %s", err.Error())
		}
		for _, addr := range addrs {
			var ip *net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = &v.IP
			case *net.IPAddr:
				ip = &v.IP
			}

			if ip == nil || ip.IsLoopback() {
				continue
			}

			if ip.To4() == nil {
				// Not an IPv4 address
				continue
			}

			logrus.Infof("Found IP: %s", ip.String())
			eth0IP = ip
			if dhclient.Process != nil {
				dhclient.Process.Kill()
			}
			break
		}
		if eth0IP == nil {
			logrus.Info("No IP address found, sleeping for 5 seconds before trying again")
			time.Sleep(5 * time.Second)
		}
	}

	logrus.Info("Sending request to http://169.254.169.254/v1.json")
	vultrMetadata, err := getMetadata()
	if err != nil {
		logrus.Fatal(err.Error())
	}

	noCloudMetadata := &NoCloudMetadata{
		AmiID:            "unknown",
		InstanceID:       vultrMetadata.InstanceID,
		Region:           vultrMetadata.Region.RegionCode,
		AvailabilityZone: "unknown",
		Tags:             []string{},
		Hostname:         vultrMetadata.Hostname,
		LocalHostname:    vultrMetadata.Hostname,
	}

	for _, publicKey := range strings.Split(vultrMetadata.PublicKeys, "\n") {
		if len(publicKey) == 0 {
			continue
		}
		noCloudMetadata.PublicKeys = append(noCloudMetadata.PublicKeys, publicKey)
	}

	err = writeMetadata(noCloudMetadata, *outputDirectory)
	if err != nil {
		logrus.Fatal(err.Error())
	}

	noCloudNetworkConfig := &NoCloudNetworkConfig{
		Version: 2,
		Config:  make([]NoCloudNetworkInterface, 0),
	}

	for i, vInterface := range vultrMetadata.Interfaces {
		if vInterface.NetworkType != "private" {
			continue
		}

		noCloudNetworkConfig.Config = append(noCloudNetworkConfig.Config, NoCloudNetworkInterface{
			Type:       "physical",
			Name:       fmt.Sprintf("eth%d", i),
			MacAddress: vInterface.Mac,
			Subnets: []NoCloudNetworkSubnet{
				{
					Type:    "static",
					Address: vInterface.IPv4.Address,
					Netmask: vInterface.IPv4.Netmask,
				},
			},
			MTU: 1450,
		})
	}

	err = writeNetworkConfig(noCloudNetworkConfig, *outputDirectory)
	if err != nil {
		logrus.Fatal(err.Error())
	}

	userdata, err := []byte{}, nil
	if err != nil {
		logrus.Fatal(err.Error())
	}

	err = writeUserData(userdata, *outputDirectory)
	if err != nil {
		logrus.Fatal(err.Error())
	}

}

func getMetadata() (*VultrMetadata, error) {
	ctx, cancel := timeoutContext()
	defer cancel()

	response, err := ctxhttp.Get(ctx, http.DefaultClient, "http://169.254.169.254/v1.json")
	if err != nil {
		return nil, fmt.Errorf("error getting metadata: %s", err.Error())
	}

	responseData, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %s", err.Error())
	}
	response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("invalid response from metadata service: %s", string(responseData))
	}

	metadata := &VultrMetadata{}
	err = json.Unmarshal(responseData, metadata)
	if err != nil {
		return nil, fmt.Errorf("error json unmarshalling metadata: %s", err.Error())
	}

	return metadata, nil
}

func writeMetadata(noCloudMetadata *NoCloudMetadata, outputDirectory string) error {
	noCloudMetadataBytes, _ := json.Marshal(noCloudMetadata)

	err := ioutil.WriteFile(path.Join(outputDirectory, "meta-data"), noCloudMetadataBytes, 0644)
	if err != nil {
		return fmt.Errorf("Error writing metadata file: %s", err.Error())
	}

	return nil
}

func writeNetworkConfig(noCloudNetworkConfig *NoCloudNetworkConfig, outputDirectory string) error {

	noCloudMetadataBytes, _ := yaml.Marshal(noCloudNetworkConfig)

	err := ioutil.WriteFile(path.Join(outputDirectory, "network-config"), noCloudMetadataBytes, 0644)
	if err != nil {
		return fmt.Errorf("Error writing network config file: %s", err.Error())
	}

	return nil
}

func writeUserData(userdata []byte, outputDirectory string) error {
	err := ioutil.WriteFile(path.Join(outputDirectory, "user-data"), userdata, 0644)
	if err != nil {
		return fmt.Errorf("Error writing metadata file: %s", err.Error())
	}

	return nil
}
