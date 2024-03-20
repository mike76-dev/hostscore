import axios from 'axios'
import { Host, NodeStatus } from './types'

const apiBaseURL = process.env.REACT_APP_API_ENDPOINT
const locations = ['eu', 'us']
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
	query: string
): Promise<{ status: string, message: string, hosts?: Host[], more: boolean, total: number }> => {
	const url = '/hosts?network=' + network +
		'&all=' + (all ? 'true' : 'false') +
		'&offset=' + offset + '&limit=' + limit +
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

export const getStatus = async (): Promise<{ status: string, message: string, version: string, nodes: NodeStatus[] }> => {
	const url = '/status'
	return instance.get(url)
	.then(response => response.data)
	.catch(error => console.log(error))
}

export * from './types'
export * from './helpers'