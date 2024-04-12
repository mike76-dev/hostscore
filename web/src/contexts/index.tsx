import { createContext } from 'react'
import { Host, HostSortType, NetworkAverages, AveragePrices } from '../api'

type NetworkContextType = {
	network: string,
	switchNetwork: (network: string) => void,
    averages: NetworkAverages,
    setAverages: (averages: NetworkAverages) => void
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

const zeroPrices: AveragePrices = {
    storagePrice: '',
    collateral: '',
    uploadPrice: '',
    downloadPrice: '',
    contractDuration: 0,
    ok: false
}

export const zeroAverages: NetworkAverages = {
    tier1: zeroPrices,
    tier2: zeroPrices,
    tier3: zeroPrices
}

export const NetworkContext = createContext<NetworkContextType>({
	network: '',
	switchNetwork: (network: string) => null,
    averages: zeroAverages,
    setAverages: (averages: NetworkAverages) => null
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