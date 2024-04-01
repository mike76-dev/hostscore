import { createContext } from 'react'
import { Host, HostSortType } from '../api'

type NetworkContextType = {
	network: string,
	switchNetwork: (network: string) => any,
}

type HostContextType = {
	hosts: Host[],
	setHosts: (hosts: Host[]) => any,
    offset: number,
    changeOffset: (offset: number) => any,
    limit: number,
    changeLimit: (limit: number) => any,
    onlineOnly: boolean,
    setOnlineOnly: (onlineOnly: boolean) => any,
    query: string,
    setQuery: (query: string) => any,
    sorting: HostSortType,
    changeSorting: (sorting: HostSortType) => any,
}

export const NetworkContext = createContext<NetworkContextType>({
	network: '',
	switchNetwork: (network: string) => {},
})

export const HostContext = createContext<HostContextType>({
	hosts: [],
	setHosts: (hosts: Host[]) => {},
    offset: 0,
    changeOffset: (offset: number) => 0,
    limit: 10,
    changeLimit: (limit: number) => 0,
    onlineOnly: true,
    setOnlineOnly: (onlineOnly: boolean) => true,
    query: '',
    setQuery: (query: string) => '',
    sorting: { sortBy: 'rank', order: 'asc' },
    changeSorting: (sorting: HostSortType) => {}
})