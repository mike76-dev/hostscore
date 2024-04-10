import axios, { CancelToken } from 'axios'
import {
	Host,
	NodeStatus,
	HostScan,
	HostBenchmark,
	PriceChange,
    HostSortType,
    NetworkAverages
} from './types'
import { LatLng } from 'leaflet'

const apiBaseURL = process.env.REACT_APP_API_ENDPOINT
const locations = ['eu', 'us', 'ap']
const excludedPaths = ['/about', '/status']

export const useLocations = () => (locations)
export const useExcludedPaths = () => (excludedPaths)

const instance = axios.create({
	baseURL: apiBaseURL,
	headers: {
		ContentType: 'application/json;charset=utf-8',
		Accept: 'application/json'
	}
})

export const getHosts = async (
	network: string,
	all: boolean,
	offset: number,
	limit: number,
	query: string,
    country: string,
    sorting: HostSortType,
    cancelToken: CancelToken
): Promise<{ status: string, message: string, hosts?: Host[], more: boolean, total: number }> => {
	const url = '/hosts?network=' + network +
		'&all=' + (all ? 'true' : 'false') +
		'&offset=' + offset + '&limit=' + limit +
		'&query=' + query +
        '&country=' + country +
        '&sort=' + sorting.sortBy +
        '&order=' + sorting.order
    return instance.get(url, { cancelToken })
	.then(response => response.data)
    .catch(error => {
        if (!axios.isCancel(error)) console.log(error)
    })
}

export const getHostsOnMap = async (
	network: string,
    northWest: LatLng,
    southEast: LatLng,
    query: string
): Promise<{ status: string, message: string, hosts?: Host[] }> => {
	const url = '/hosts/map?network=' + network +
		'&from=' + northWest.lat.toFixed(2) + ',' + northWest.lng.toFixed(2) +
		'&to=' + southEast.lat.toFixed(2) + ',' + southEast.lng.toFixed(2) +
        '&query=' + query
	return instance.get(url)
	.then(response => response.data)
	.catch(error => console.log(error))
}

export const getHost = async (
	network: string,
	publicKey: string
): Promise<{ status: string, message: string, host?: Host }> => {
	const url = '/host?network=' + network + '&host=' + publicKey
	return instance.get(url)
	.then(response => response.data)
	.catch(error => console.log(error))
}

export const getStatus = async ():
	Promise<{ status: string, message: string, version: string, nodes: NodeStatus[] }> => {
	const url = '/status'
	return instance.get(url)
	.then(response => response.data)
	.catch(error => console.log(error))
}

export const getOnlineHosts = async (network: string):
	Promise<{ status: string, message: string, onlineHosts: number }> => {
	const url = '/hosts/online?network=' + network
	return instance.get(url)
	.then(response => response.data)
	.catch(error => console.log(error))
}

export const getScans = async (
	network: string,
	publicKey: string,
	from?: Date,
	to?: Date,
	num?: number,
	success?: boolean
): Promise<{ status: string, message: string, scans: HostScan[] }> => {
	const url = '/scans?network=' + network + '&host=' + publicKey +
		(from ? '&from=' + from.toISOString() : '') +
		(to ? '&to=' + to.toISOString() : '') +
		(num ? '&number=' + num : '') + (success ? '&success=true' : '')
	return instance.get(url)
	.then(response => response.data)
	.catch(error => console.log(error))
}

export const getBenchmarks = async (
	network: string,
	publicKey: string,
	from?: Date,
	to?: Date,
	num?: number,
	success?: boolean
): Promise<{ status: string, message: string, benchmarks: HostBenchmark[] }> => {
	const url = '/benchmarks?network=' + network + '&host=' + publicKey +
		(from ? '&from=' + from.toISOString() : '') +
		(to ? '&to=' + to.toISOString() : '') +
		(num ? '&number=' + num : '') + (success ? '&success=true' : '')
	return instance.get(url)
	.then(response => response.data)
	.catch(error => console.log(error))
}

export const getPriceChanges = async (
	network: string,
	publicKey: string
): Promise<{ status: string, message: string, priceChanges: PriceChange[] }> => {
	const url = '/changes?network=' + network + '&host=' + publicKey
	return instance.get(url)
	.then(response => response.data)
	.catch(error => console.log(error))
}

export const getAverages = async (network: string):
	Promise<{ status: string, message: string, averages: NetworkAverages }> => {
	const url = '/averages?network=' + network
	return instance.get(url)
	.then(response => response.data)
	.catch(error => console.log(error))
}

export const getCountries = async (network: string):
	Promise<{ status: string, message: string, countries: string[] }> => {
	const url = '/countries?network=' + network
	return instance.get(url)
	.then(response => response.data)
	.catch(error => console.log(error))
}

export * from './types'
export * from './helpers'