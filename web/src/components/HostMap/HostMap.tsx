import './HostMap.css'
import { Host } from '../../api'
import {
    MapContainer,
    TileLayer,
    Marker,
    Popup
} from 'react-leaflet'
import { LatLngExpression } from 'leaflet'

type HostMapProps = {
    darkMode: boolean,
    host: Host
}

export const HostMap = (props: HostMapProps) => {
    const geolocation = (location: string) => {
        return location.split(',').map(l => Number.parseFloat(l)) as LatLngExpression
    }
    return (
        <div className={'host-map-container' + (props.darkMode ? ' host-map-dark' : '')}>
            <MapContainer
                center={geolocation(props.host.loc)}
                zoom={7}
                scrollWheelZoom={false}>
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
                <Marker position={geolocation(props.host.loc)}>
                    <Popup className="host-map-popup">
                        {props.host.city + ', ' + props.host.region + ', ' + props.host.country}
                    </Popup>
                </Marker>
            </MapContainer>
        </div>
    )
}