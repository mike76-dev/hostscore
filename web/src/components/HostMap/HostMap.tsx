import './HostMap.css'
import { useState, useEffect } from 'react'
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
    divIcon
} from 'leaflet'

type HostMapProps = {
    darkMode: boolean,
    network: string,
    host?: Host,
    hosts?: Host[]
}

const defaultLocation = '52.37,5.22'

const UpdateMap = () => {
    const map = useMap()
    useEffect(() => {
        map.invalidateSize()
    })
    return null
}

export const HostMap = (props: HostMapProps) => {
    const [center, setCenter] = useState(defaultLocation)
    useEffect(() => {
        if (!props.host && navigator.geolocation) {
            navigator.geolocation.getCurrentPosition(
                async (pos: GeolocationPosition) => {
                    setCenter('' + pos.coords.latitude + ',' + pos.coords.longitude)
                }
            )
        }
    }, [props.host])
    const geolocation = (location: string) => {
        return location.split(',').map(l => Number.parseFloat(l)) as LatLngExpression
    }
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
                            attribution={props.darkMode ?
                                '&copy; <a href="https://carto.com/attributions">Carto</a> contributors' :
                                '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors'
                            }
                            url={props.darkMode ?
                                'https://{s}.basemaps.cartocdn.com/dark_all/{z}/{x}/{y}{r}.png' :
                                'https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png'
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
                    center={geolocation(center)}
                    zoom={7}
                    scrollWheelZoom={true}
                >
                    <TileLayer
                        attribution={props.darkMode ?
                            '&copy; <a href="https://carto.com/attributions">Carto</a> contributors' :
                            '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors'
                        }
                        url={props.darkMode ?
                            'https://{s}.basemaps.cartocdn.com/dark_all/{z}/{x}/{y}{r}.png' :
                            'https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png'
                        }
                    />
                    {props.hosts.map(host => (
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
                </MapContainer>
            }
        </div>
    )
}