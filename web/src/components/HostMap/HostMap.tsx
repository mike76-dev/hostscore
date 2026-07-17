import './HostMap.css'
import { useState, useEffect, useRef } from 'react'
import { Link } from 'react-router-dom'
import {
	Host,
	stripePrefix
} from '../../api'
import {
	MapContainer,
	TileLayer,
	Marker,
	Popup,
	useMap
} from 'react-leaflet'
import {
	LatLngExpression,
	latLngBounds,
	divIcon
} from 'leaflet'

type HostMapProps = {
	darkMode: boolean,
	network: string,
	host?: Host,
	hosts?: Host[],
	filtered?: boolean
}

const defaultLocation = '52.37,5.22'.split(',').map(coord => parseFloat(coord)) as [number, number]

const geolocation = (location: string) => {
	return location.split(',').map(l => Number.parseFloat(l)) as LatLngExpression
}

const UpdateMap = () => {
	const map = useMap()
	useEffect(() => {
		map.invalidateSize()
	})
	return null
}

interface CenterProps {
	center: LatLngExpression,
	zoom: number
}

const CenterMap = (props: CenterProps) => {
	const map = useMap()
	useEffect(() => {
		if (props.center) map.setView(props.center, props.zoom)
	}, [map, props.center, props.zoom])
	return null
}

interface FitProps {
	hosts: Host[],
	filtered: boolean
}

// Fits the viewport to the current host list whenever a search or country
// filter produces a new list (and once more when the filter is cleared).
const FitHosts = ({ hosts, filtered }: FitProps) => {
	const map = useMap()
	const prevHosts = useRef(hosts)
	const wasFiltered = useRef(false)
	useEffect(() => {
		if (prevHosts.current === hosts) return
		prevHosts.current = hosts
		const shouldFit = filtered || wasFiltered.current
		wasFiltered.current = filtered
		if (!shouldFit) return
		const located = hosts.filter(host => host.loc !== '')
		if (located.length === 0) return
		map.fitBounds(
			latLngBounds(located.map(host => geolocation(host.loc))),
			{ padding: [40, 40], maxZoom: 10 }
		)
	}, [map, hosts, filtered])
	return null
}

export const HostMap = (props: HostMapProps) => {
	const [center, setCenter] = useState<LatLngExpression>(defaultLocation)
	const zoom = 7
	useEffect(() => {
		if (!props.host && navigator.geolocation) {
			navigator.geolocation.getCurrentPosition(
				async (pos: GeolocationPosition) => {
					setCenter([pos.coords.latitude, pos.coords.longitude])
				}
			)
		}
	}, [props.host])
	const newLocation = (host: Host) => {
		let href = window.location.href
		if (href[href.length - 1] === '/') {
			return href + 'host/' + stripePrefix(host.publicKey)
		}
		return href + '/host/' + stripePrefix(host.publicKey)
	}
	return (
		<div className={'host-map-container' + (props.darkMode ? ' host-map-dark' : '')}>
			{props.host &&
				(props.host.loc !== '' ?
					<MapContainer
						center={geolocation(props.host.loc)}
						zoom={7}
						scrollWheelZoom={true}
					>
						<TileLayer
							attribution='&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors &copy; <a href="https://carto.com/attributions">CARTO</a>'
							url={props.darkMode ?
								'https://{s}.basemaps.cartocdn.com/dark_all/{z}/{x}/{y}{r}.png' :
								'https://{s}.basemaps.cartocdn.com/light_all/{z}/{x}/{y}{r}.png'
							}
						/>
						<Marker
							position={geolocation(props.host.loc)}
							icon={divIcon({className: 'host-map-marker'})}
						>
							<Popup className="host-map-popup">
								{props.host.city + ', ' + props.host.region + ', ' + props.host.country}
							</Popup>
						</Marker>
						<UpdateMap/>
					</MapContainer>
				: <div className="host-map-unknown">Location unknown</div>
				)
			}
			{props.hosts &&
				<MapContainer
					center={center}
					zoom={zoom}
					scrollWheelZoom={true}
				>
					<TileLayer
						attribution='&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors &copy; <a href="https://carto.com/attributions">CARTO</a>'
						url={props.darkMode ?
							'https://{s}.basemaps.cartocdn.com/dark_all/{z}/{x}/{y}{r}.png' :
							'https://{s}.basemaps.cartocdn.com/light_all/{z}/{x}/{y}{r}.png'
						}
					/>
					{props.hosts.map(host => (
						host.loc !== '' &&
						<Marker
							key={host.publicKey}
							position={geolocation(host.loc)}
							icon={divIcon({className: 'host-map-marker'})}
						>
							<Popup className="host-map-popup">
								<Link className="host-map-link" to={newLocation(host)}>
									{host.netaddress}
								</Link>
							</Popup>
						</Marker>
					))}
					<UpdateMap/>
					<CenterMap center={center} zoom={zoom}/>
					<FitHosts
						hosts={props.hosts}
						filtered={props.filtered === true}
					/>
				</MapContainer>
			}
		</div>
	)
}