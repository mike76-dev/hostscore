import './Averages.css'
import { Tooltip } from '../'
import {
    NetworkAverages,
    convertPriceRaw
} from '../../api'

type AveragesProps = {
    darkMode: boolean,
    averages: NetworkAverages
}

const AveragesTooltip = () => (
    <div>
        The prices given here do not count for any redundancy.
        They are given from the hosts' perspective.
    </div>
)

export const Averages = (props: AveragesProps) => {
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
            <div>Network Averages
                <Tooltip className="averages-tooltip" darkMode={props.darkMode}>
                    <AveragesTooltip/>
                </Tooltip>
            </div>
            <table>
                <tbody>
                    {props.averages.tier1.ok === true &&
                        <>
                            <tr><th colSpan={2}>1st Tier (Top 10)</th></tr>
                            <tr>
                                <td>Storage Price</td>
                                <td>{toSia(convertPriceRaw(props.averages.tier1.storagePrice) * 144 * 30) + '/TB/month'}</td>
                            </tr>
                            <tr>
                                <td>Collateral</td>
                                <td>{toSia(convertPriceRaw(props.averages.tier1.collateral) * 144 * 30) + '/TB/month'}</td>
                            </tr>
                            <tr>
                                <td>Upload Price</td>
                                <td>{toSia(convertPriceRaw(props.averages.tier1.uploadPrice)) + '/TB'}</td>
                            </tr>
                            <tr>
                                <td>Download Price</td>
                                <td>{toSia(convertPriceRaw(props.averages.tier1.downloadPrice)) + '/TB'}</td>
                            </tr>
                        </>
                    }
                    {props.averages.tier2.ok === true &&
                        <>
                            <tr><th colSpan={2}>2nd Tier (Top 100 Minus Tier 1)</th></tr>
                            <tr>
                                <td>Storage Price</td>
                                <td>{toSia(convertPriceRaw(props.averages.tier2.storagePrice) * 144 * 30) + '/TB/month'}</td>
                            </tr>
                            <tr>
                                <td>Collateral</td>
                                <td>{toSia(convertPriceRaw(props.averages.tier2.collateral) * 144 * 30) + '/TB/month'}</td>
                            </tr>
                            <tr>
                                <td>Upload Price</td>
                                <td>{toSia(convertPriceRaw(props.averages.tier2.uploadPrice)) + '/TB'}</td>
                            </tr>
                            <tr>
                                <td>Download Price</td>
                                <td>{toSia(convertPriceRaw(props.averages.tier2.downloadPrice)) + '/TB'}</td>
                            </tr>
                        </>
                    }
                    {props.averages.tier3.ok === true &&
                        <>
                            <tr><th colSpan={2}>3rd Tier (The Rest)</th></tr>
                            <tr>
                                <td>Storage Price</td>
                                <td>{toSia(convertPriceRaw(props.averages.tier3.storagePrice) * 144 * 30) + '/TB/month'}</td>
                            </tr>
                            <tr>
                                <td>Collateral</td>
                                <td>{toSia(convertPriceRaw(props.averages.tier3.collateral) * 144 * 30) + '/TB/month'}</td>
                            </tr>
                            <tr>
                                <td>Upload Price</td>
                                <td>{toSia(convertPriceRaw(props.averages.tier3.uploadPrice)) + '/TB'}</td>
                            </tr>
                            <tr>
                                <td>Download Price</td>
                                <td>{toSia(convertPriceRaw(props.averages.tier3.downloadPrice)) + '/TB'}</td>
                            </tr>
                        </>
                    }
                </tbody>
            </table>
        </div>
    )
}