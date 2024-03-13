import { createContext } from 'react'
import { Host } from '../api'

type NetworkContextType = {
	network: string,
	switchNetwork: (network: string) => any,
}

type HostContextType = {
	hosts: Host[],
	setHosts: (hosts: Host[]) => any,
}

export const NetworkContext = createContext<NetworkContextType>({
	network: '',
	switchNetwork: (network: string) => {},
})

export const HostContext = createContext<HostContextType>({
	hosts: [],
	setHosts: (hosts: Host[]) => {},
})