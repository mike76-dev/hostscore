import './Averages.css'
import { Tooltip } from '../'
import {
    NetworkAverages,
    blocksToTime,
    toSia
} from '../../api'

type AveragesProps = {
    darkMode: boolean,
    averages: { [tier: string]: NetworkAverages }
}

const AveragesTooltip = () => (
    <div>
        The prices given here do not count for any redundancy.
        They are given from the hosts' perspective.
    </div>
)

export const Averages = (props: AveragesProps) => {
    return (
        <div className={'averages-container' + (props.darkMode ? ' averages-dark' : '')}>
            <div><strong>Network Averages</strong>
                <Tooltip className="averages-tooltip" darkMode={props.darkMode}>
                    <AveragesTooltip/>
                </Tooltip>
            </div>
            <table>
                <tbody>
                    {props.averages['tier1'] && props.averages['tier1'].available === true &&
                        <>
                            <tr><th colSpan={2}>1st Tier (Top 10)</th></tr>
                            <tr>
                                <td>Storage Price</td>
                                <td>{toSia(props.averages['tier1'].storagePrice, true) + '/TB/month'}</td>
                            </tr>
                            <tr>
                                <td>Collateral</td>
                                <td>{toSia(props.averages['tier1'].collateral, true) + '/TB/month'}</td>
                            </tr>
                            <tr>
                                <td>Upload Price</td>
                                <td>{toSia(props.averages['tier1'].uploadPrice, false) + '/TB'}</td>
                            </tr>
                            <tr>
                                <td>Download Price</td>
                                <td>{toSia(props.averages['tier1'].downloadPrice, false) + '/TB'}</td>
                            </tr>
                            <tr>
                                <td>Contract Duration</td>
                                <td>{blocksToTime(props.averages['tier1'].contractDuration)}</td>
                            </tr>
                        </>
                    }
                    {props.averages['tier2'] && props.averages['tier2'].available === true &&
                        <>
                            <tr><th colSpan={2}>2nd Tier (Top 100 Minus Tier 1)</th></tr>
                            <tr>
                                <td>Storage Price</td>
                                <td>{toSia(props.averages['tier2'].storagePrice, true) + '/TB/month'}</td>
                            </tr>
                            <tr>
                                <td>Collateral</td>
                                <td>{toSia(props.averages['tier2'].collateral, true) + '/TB/month'}</td>
                            </tr>
                            <tr>
                                <td>Upload Price</td>
                                <td>{toSia(props.averages['tier2'].uploadPrice, false) + '/TB'}</td>
                            </tr>
                            <tr>
                                <td>Download Price</td>
                                <td>{toSia(props.averages['tier2'].downloadPrice, false) + '/TB'}</td>
                            </tr>
                            <tr>
                                <td>Contract Duration</td>
                                <td>{blocksToTime(props.averages['tier2'].contractDuration)}</td>
                            </tr>
                        </>
                    }
                    {props.averages['tier3'] && props.averages['tier3'].available === true &&
                        <>
                            <tr><th colSpan={2}>3rd Tier (The Rest)</th></tr>
                            <tr>
                                <td>Storage Price</td>
                                <td>{toSia(props.averages['tier3'].storagePrice, true) + '/TB/month'}</td>
                            </tr>
                            <tr>
                                <td>Collateral</td>
                                <td>{toSia(props.averages['tier3'].collateral, true) + '/TB/month'}</td>
                            </tr>
                            <tr>
                                <td>Upload Price</td>
                                <td>{toSia(props.averages['tier3'].uploadPrice, false) + '/TB'}</td>
                            </tr>
                            <tr>
                                <td>Download Price</td>
                                <td>{toSia(props.averages['tier3'].downloadPrice, false) + '/TB'}</td>
                            </tr>
                            <tr>
                                <td>Contract Duration</td>
                                <td>{blocksToTime(props.averages['tier3'].contractDuration)}</td>
                            </tr>
                        </>
                    }
                </tbody>
            </table>
        </div>
    )
}