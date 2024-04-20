import { createContext } from 'react'
import { Host, HostSortType, NetworkAverages } from '../api'

type NetworkContextType = {
	network: string,
	switchNetwork: (network: string) => void,
    averages: { [tier: string]: NetworkAverages },
    setAverages: (averages: { [tier: string]: NetworkAverages }) => void
}

type HostContextType = {
	hosts: Host[],
	setHosts: (hosts: Host[]) => void,
    offset: number,
    changeOffset: (offset: number) => void,
    limit: number,
    changeLimit: (limit: number) => void,
    onlineOnly: boolean,
    setOnlineOnly: (onlineOnly: boolean) => void,
    query: string,
    setQuery: (query: string) => void,
    sorting: HostSortType,
    changeSorting: (sorting: HostSortType) => void,
    countries: string[],
    setCountries: (countries: string[]) => void,
    country: string,
    setCountry: (country: string) => void,
}

export const NetworkContext = createContext<NetworkContextType>({
	network: '',
	switchNetwork: (network: string) => null,
    averages: {},
    setAverages: (averages: { [tier: string]: NetworkAverages }) => null
})

export const HostContext = createContext<HostContextType>({
	hosts: [],
	setHosts: (hosts: Host[]) => null,
    offset: 0,
    changeOffset: (offset: number) => null,
    limit: 10,
    changeLimit: (limit: number) => null,
    onlineOnly: true,
    setOnlineOnly: (onlineOnly: boolean) => null,
    query: '',
    setQuery: (query: string) => null,
    sorting: { sortBy: 'rank', order: 'asc' },
    changeSorting: (sorting: HostSortType) => null,
    countries: [],
    setCountries: (countries: string[]) => null,
    country: '',
    setCountry: (country: string) => null
})