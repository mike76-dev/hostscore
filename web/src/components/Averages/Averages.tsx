import './Averages.css'
import { useState, useEffect } from 'react'
import { Host, convertPriceRaw } from '../../api'

type AveragesProps = {
    darkMode: boolean,
    hosts: Host[]
}

type AveragePrices = {
    storage: number,
    collateral: number,
    upload: number,
    download: number
}

const initialValues: AveragePrices = {
    storage: 0,
    collateral: 0,
    upload: 0,
    download: 0
}

export const Averages = (props: AveragesProps) => {
    const [averages1, setAverages1] = useState<AveragePrices>(initialValues)
    const [averages2, setAverages2] = useState<AveragePrices>(initialValues)
    const [averages3, setAverages3] = useState<AveragePrices>(initialValues)
    useEffect(() => {
        const tier1 = props.hosts.slice(0, 10)
        let av1 = structuredClone(initialValues)
        tier1.forEach(host => {
            av1.storage += convertPriceRaw(host.settings.storageprice)
            av1.collateral += convertPriceRaw(host.settings.collateral)
            av1.upload += convertPriceRaw(host.settings.uploadbandwidthprice)
            av1.download += convertPriceRaw(host.settings.downloadbandwidthprice)
        })
        if (tier1.length > 0) {
            av1.storage /= tier1.length
            av1.collateral /= tier1.length
            av1.upload /= tier1.length
            av1.download /= tier1.length
        }
        setAverages1(av1)
        const tier2 = props.hosts.slice(10, 100)
        let av2 = structuredClone(initialValues)
        tier2.forEach(host => {
            av2.storage += convertPriceRaw(host.settings.storageprice)
            av2.collateral += convertPriceRaw(host.settings.collateral)
            av2.upload += convertPriceRaw(host.settings.uploadbandwidthprice)
            av2.download += convertPriceRaw(host.settings.downloadbandwidthprice)
        })
        if (tier2.length > 0) {
            av2.storage /= tier2.length
            av2.collateral /= tier2.length
            av2.upload /= tier2.length
            av2.download /= tier2.length
        }
        setAverages2(av2)
        const tier3 = props.hosts.slice(100)
        let av3 = structuredClone(initialValues)
        tier3.forEach(host => {
            av3.storage += convertPriceRaw(host.settings.storageprice)
            av3.collateral += convertPriceRaw(host.settings.collateral)
            av3.upload += convertPriceRaw(host.settings.uploadbandwidthprice)
            av3.download += convertPriceRaw(host.settings.downloadbandwidthprice)
        })
        if (tier3.length > 0) {
            av3.storage /= tier3.length
            av3.collateral /= tier3.length
            av3.upload /= tier3.length
            av3.download /= tier3.length
        }
        setAverages3(av3)
    }, [props.hosts])
    const toSia = (price: number) => {
        if (price < 1e-12) return '0 H'
        if (price < 1e-9) return (price * 1000).toFixed(0) + ' pS'
        if (price < 1e-6) return (price * 1000).toFixed(0) + ' nS'
        if (price < 1e-3) return (price * 1000).toFixed(0) + ' uS'
        if (price < 1) return (price * 1000).toFixed(0) + ' mS'
        if (price < 10) return price.toFixed(1) + ' SC'
        if (price < 1e3) return price.toFixed(0) + ' SC'
        if (price < 1e4) return (price / 1000).toFixed(1) + ' KS'
        return (price / 1000).toFixed(0) + ' KS'
    }
    return (
        <div className={'averages-container' + (props.darkMode ? ' averages-dark' : '')}>
            <p>Network Averages</p>
            <table>
                <tbody>
                    <tr><th colSpan={2}>1st Tier (Top 10)</th></tr>
                    <tr>
                        <td>Storage Price</td>
                        <td>{toSia(averages1.storage * 144 * 30) + '/TB/month'}</td>
                    </tr>
                    <tr>
                        <td>Collateral</td>
                        <td>{toSia(averages1.collateral * 144 * 30) + '/TB/month'}</td>
                    </tr>
                    <tr>
                        <td>Upload Price</td>
                        <td>{toSia(averages1.upload) + '/TB'}</td>
                    </tr>
                    <tr>
                        <td>Download Price</td>
                        <td>{toSia(averages1.download) + '/TB'}</td>
                    </tr>
                    {props.hosts.length > 10 &&
                        <>
                            <tr><th colSpan={2}>2nd Tier (Top 100 Minus Tier 1)</th></tr>
                            <tr>
                                <td>Storage Price</td>
                                <td>{toSia(averages2.storage * 144 * 30) + '/TB/month'}</td>
                            </tr>
                            <tr>
                                <td>Collateral</td>
                                <td>{toSia(averages2.collateral * 144 * 30) + '/TB/month'}</td>
                            </tr>
                            <tr>
                                <td>Upload Price</td>
                                <td>{toSia(averages2.upload) + '/TB'}</td>
                            </tr>
                            <tr>
                                <td>Download Price</td>
                                <td>{toSia(averages2.download) + '/TB'}</td>
                            </tr>
                        </>
                    }
                    {props.hosts.length > 100 &&
                        <>
                            <tr><th colSpan={2}>3rd Tier (The Rest)</th></tr>
                            <tr>
                                <td>Storage Price</td>
                                <td>{toSia(averages3.storage * 144 * 30) + '/TB/month'}</td>
                            </tr>
                            <tr>
                                <td>Collateral</td>
                                <td>{toSia(averages3.collateral * 144 * 30) + '/TB/month'}</td>
                            </tr>
                            <tr>
                                <td>Upload Price</td>
                                <td>{toSia(averages3.upload) + '/TB'}</td>
                            </tr>
                            <tr>
                                <td>Download Price</td>
                                <td>{toSia(averages3.download) + '/TB'}</td>
                            </tr>
                        </>
                    }
                </tbody>
            </table>
        </div>
    )
}