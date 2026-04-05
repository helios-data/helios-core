package core

import (

)

type LocalInterface struct {
	runtimeHash string
}

func initializeLocalClient(hash string) *LocalInterface {
	return &LocalInterface{
		runtimeHash: hash,
	}
}

func (x *LocalInterface) Initialize() {

}

func (x *LocalInterface) InitializeComponentTree(path string) {

}

func (x *LocalInterface) StartComponent(name string) {

}

func (x *LocalInterface) StartAllComponents() {

}

func (x *LocalInterface) StopComponent(name string) {

}

func (x *LocalInterface) KillComponent(name string) {

}

func (x *LocalInterface) Clean() {

}

func (x *LocalInterface) Close() {

}
