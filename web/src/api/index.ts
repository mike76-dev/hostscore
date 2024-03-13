import axios from 'axios'
import { Host } from './types'

const apiBaseURL = process.env.REACT_APP_API_ENDPOINT
const locations = ['EU']

export const useLocations = () => (locations)

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
	const location = locations[0]
	const url = '/hosts?location=' +
		location + '&network=' + network +
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
	const location = locations[0]
	const url = '/host?location=' +
		location + '&network=' + network +
		'&host=' + publicKey
	return instance.get(url)
	.then(response => response.data)
	.catch(error => console.log(error))
}

export * from './types'
export * from './helpers'