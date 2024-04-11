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
import L, {
    LatLngExpression,
    LatLngBounds,
    divIcon
} from 'leaflet'

type HostMapProps = {
    darkMode: boolean,
    network: string,
    host?: Host,
    hosts?: Host[]
}

const defaultLocation = '52.37,5.22'.split(',').map(coord => parseFloat(coord)) as [number, number]

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

interface BoundsProps {
    network: string,
    hosts?: Host[],
    onBoundsChange: (bounds: LatLngBounds) => void,
    setMapWidth: (width: number) => void,
    setMapHeight: (height: number) => void,
}

const Bounds: React.FC<BoundsProps> = ({
    network,
    hosts,
    onBoundsChange,
    setMapWidth,
    setMapHeight
    }) => {
    const map = useMap()
    useEffect(() => {
        const mc = map.getContainer()
        setMapWidth(mc.clientWidth)
        setMapHeight(mc.clientHeight)
        onBoundsChange(map.getBounds())
        const updateBounds = () => {
            onBoundsChange(map.getBounds())
        }
        map.on('moveend', updateBounds)
        map.on('zoomend', updateBounds)
        return () => {
            map.off('moveend', updateBounds)
            map.off('zoomend', updateBounds)
        }
    // eslint-disable-next-line
    }, [map, network, hosts])
    return null
}

export const HostMap = (props: HostMapProps) => {
    const [center, setCenter] = useState<LatLngExpression>(defaultLocation)
    const [bounds, setBounds] = useState<LatLngBounds | undefined>()
    const [zoom, setZoom] = useState(7)
    const [mapWidth, setMapWidth] = useState(0)
    const [mapHeight, setMapHeight] = useState(0)
    useEffect(() => {
        if (!props.host && navigator.geolocation) {
            navigator.geolocation.getCurrentPosition(
                async (pos: GeolocationPosition) => {
                    setCenter([pos.coords.latitude, pos.coords.longitude])
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
    const handleBoundsChange = (b: LatLngBounds) => {
        setBounds(b)
    }
    useEffect(() => {
        if (!bounds || !props.hosts || props.hosts.length === 0)  return
        let minLat = 90
        let maxLat = -90
        let minLng = 180
        let maxLng = -180
        props.hosts.forEach(host => {
            let loc = host.loc.split(',').map(l => Number.parseFloat(l))
            if (loc[0] < minLat) minLat = loc[0]
            if (loc[0] > maxLat) maxLat = loc[0]
            if (loc[1] < minLng) minLng = loc[1]
            if (loc[1] > maxLng) maxLng = loc[1]
        })
        let deltaLat = maxLat - minLat
        let deltaLng = maxLng - minLng
        if (deltaLng >= 180) deltaLng = 360 - deltaLng
        let centerLat = (maxLat + minLat) / 2
        let centerLng = (maxLng + minLng) / 2
        let ne = bounds.getNorthEast()
        let sw = bounds.getSouthWest()
        let ns = ne.lat - sw.lat
        let ew = ne.lng - sw.lng
        if (ew >= 180) ew = 360 - ew
        let newZoom = zoom
        do {
            if (deltaLat <= ns * (zoom - newZoom + 1) && deltaLng <= ew * (zoom - newZoom + 1)) {
                if ((center as [number, number])[0] !== centerLat && (center as [number, number])[1] !== centerLng) {
                    setZoom(newZoom)
                    setCenter([centerLat, centerLng])
                    break
                }
            }
            newZoom--
        } while (newZoom > 5)
    }, [props.hosts, bounds, setCenter])
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
                    center={center}
                    zoom={zoom}
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
                    <CenterMap center={center} zoom={zoom}/>
                    <Bounds
                        network={props.network}
                        hosts={props.hosts}
                        onBoundsChange={handleBoundsChange}
                        setMapWidth={setMapWidth}
                        setMapHeight={setMapHeight}
                    />
                </MapContainer>
            }
        </div>
    )
}