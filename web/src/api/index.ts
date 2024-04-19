import axios, { CancelToken } from 'axios'
import {
	Host,
	NodeStatus,
	PriceChange,
    HostSortType,
    NetworkAverages
} from './types'

const apiBaseURL = process.env.REACT_APP_API_ENDPOINT
const locations = [
    { short: 'europe', long: 'Europe' },
    { short: 'east-us', long: 'East USA' },
    { short: 'asia', long: 'Asia' }
]
const excludedPaths = ['/about', '/faq', '/status']

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
	Promise<{ nodes: { [node: string]: NodeStatus }, version: string }> => {
	const url = '/service/status'
	return instance.get(url)
	.then(response => { console.log(response); return response.data})
	.catch(error => console.log(error))
}

export const getOnlineHosts = async (network: string):
	Promise<{ status: string, message: string, onlineHosts: number }> => {
	const url = '/hosts/online?network=' + network
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

export const getCountries = async (network: string, all: boolean):
	Promise<{ countries: string[] }> => {
	const url = '/network/countries?network=' + network + (all ? '' : '&all=false')
	return instance.get(url)
	.then(response => {
        if (response.status === 200) return response.data
        else console.log(response.statusText)
    })
	.catch(error => console.log(error))
}

export * from './types'
export * from './helpers'