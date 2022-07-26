/*
Copyright © 2022 NAME HERE <EMAIL ADDRESS>

*/
package utils

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	typedv1 "k8s.io/client-go/kubernetes/typed/core/v1"
)

func CreateConfigmap(ctx context.Context, core typedv1.CoreV1Interface, name, dir, namespace string) (string, error) {
	// Create a bundle configmap
	data := map[string]string{}
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		c, err := ioutil.ReadFile(path)
		if err != nil {
			return err
		}
		data[info.Name()] = string(c)
		return nil
	})
	if err != nil {
		return "", err
	}
	configmap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: name,
			Namespace:    namespace,
		},
		Data: data,
	}
	configmap, err = core.ConfigMaps(namespace).Create(ctx, configmap, metav1.CreateOptions{})
	return configmap.ObjectMeta.Name, err
}
