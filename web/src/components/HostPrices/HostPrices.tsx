import './HostPrices.css'
import { useRef, useState, useEffect } from 'react'
import { PriceChange, convertPriceRaw } from '../../api'
import {
    Chart,
    ChartData,
    CategoryScale,
    Filler,
    LinearScale,
    LineController,
    LineElement,
    PointElement,
    Legend
} from 'chart.js'
import { Controls, ScaleOptions } from './Controls/Controls'

Chart.register(
    CategoryScale,
    Filler,
    LinearScale,
    LineElement,
    LineController,
    PointElement,
    Legend
)

type HostPricesProps = {
    darkMode: boolean,
    data: PriceChange[]
}

type Dataset = {
    data: number[],
    label: string,
    yAxisID: string,
    borderColor: string,
    backgroundColor: string,
    fill: boolean | string,
    stepped: boolean | string,
    pointRadius: number,
    borderWidth: number,
    order: number
}

type PriceChartProps = {
    data: PriceChange[],
    scale: ScaleOptions,
    setScale: (scale: ScaleOptions) => any,
    maxTimestamp: number,
    setMaxTimestamp: (maxTimestamp: number) => any,
    darkMode: boolean
}

const formatLabel = (point: Date, scale: ScaleOptions) => {
    let res = point.toLocaleDateString()
    switch (scale) {
        case 'day':
            let prefix = point.getHours() < 2 ? '' + res.slice(0, res.length - 5) + ' ' : ''
            return prefix + point.getHours() + ':00'
        case 'week':
            let suffix = point.getDate() === 1 && point.getMonth() === 0 ? res.slice(res.length - 5) : ''
            return res.slice(0, res.length - 5) + suffix
        case 'month':
            let suffix1 = point.getDate() <= 3 && point.getMonth() === 0 ? res.slice(res.length - 5) : ''
            return res.slice(0, res.length - 5) + suffix1
        case 'year':
            return '' + (point.getMonth() + 1) + '-' + point.getFullYear()
        default:
            return ''
    }
}

const scaling = (data: PriceChange[], maxTimestamp: number, scale: ScaleOptions) => {
    let max = new Date(maxTimestamp)
    max.setMinutes(0)
    max.setSeconds(0)
    let min = new Date(maxTimestamp)
    min.setMinutes(0)
    min.setSeconds(0)
    let num = 1
    switch (scale) {
        case 'day':
            min.setDate(max.getDate() - 1)
            num = 12
            break
        case 'week':
            min.setDate(max.getDate() - 7)
            num = 7
            break
        case 'month':
            min.setMonth(max.getMonth() - 1)
            num = 10
            break
        case 'year':
            min.setFullYear(max.getFullYear() - 1)
            num = 12
            break
        default:
    }
    let int = Math.floor((max.getTime() - min.getTime()) / num)
    return {
        minValue: min,
        maxValue: max,
        numPoints: num,
        interval: int
    }
}

const newTimestamp = (data: PriceChange[], maxTimestamp: number, scale: ScaleOptions, forward: boolean) => {
    const { maxValue, interval } = scaling(data, maxTimestamp, scale)
    let oldTimestamp = maxValue.getTime()
    return forward ? oldTimestamp + interval : oldTimestamp - interval
}

const PriceChart = (props: PriceChartProps) => {
    const formatData = (data: PriceChange[]): ChartData => {
        if (data.length === 0) return { labels: [], datasets: [] }
        const { minValue, numPoints, interval } = scaling(data, props.maxTimestamp, props.scale)
        let datasets: Dataset[] = []
        let labels: string[] = []
        let remainingStorage: number[] = []
        let totalStorage: number[] = []
        let uploadPrice: number[] = []
        let downloadPrice: number[] = []
        let storagePrice: number[] = []
        let collateral: number[] = []
        let rs = 0
        let ts = 0
        let up = 0
        let dp = 0
        let sp = 0
        let col = 0
        let start = 0
        for (let i = 0; i < props.data.length; i++) {
            if (minValue.getTime() < (new Date(props.data[i].timestamp)).getTime()) break
            rs = props.data[i].remainingStorage / 1e12
            ts = props.data[i].totalStorage / 1e12
            up = convertPriceRaw(props.data[i].uploadPrice)
            dp = convertPriceRaw(props.data[i].downloadPrice)
            sp = convertPriceRaw(props.data[i].storagePrice) * 144 * 30
            col = convertPriceRaw(props.data[i].collateral) * 144 * 30
            start = i
        }
        remainingStorage.push(rs)
        totalStorage.push(ts)
        uploadPrice.push(up)
        downloadPrice.push(dp)
        storagePrice.push(sp)
        collateral.push(col)
        labels.push(formatLabel(minValue, props.scale))
        for (let i = 0; i < numPoints; i++) {
            minValue.setTime(minValue.getTime() + interval)
            for (let j = start; j < props.data.length; j++) {
                if (minValue.getTime() < (new Date(props.data[j].timestamp)).getTime()) break
                rs = props.data[j].remainingStorage / 1e12
                ts = props.data[j].totalStorage / 1e12
                up = convertPriceRaw(props.data[j].uploadPrice)
                dp = convertPriceRaw(props.data[j].downloadPrice)
                sp = convertPriceRaw(props.data[j].storagePrice) * 144 * 30
                col = convertPriceRaw(props.data[j].collateral) * 144 * 30
                start = j
            }
            remainingStorage.push(rs)
            totalStorage.push(ts)
            uploadPrice.push(up)
            downloadPrice.push(dp)
            storagePrice.push(sp)
            collateral.push(col)
            labels.push(formatLabel(minValue, props.scale))
        }
        datasets.push({
            data: totalStorage,
            label: 'Total Storage',
            yAxisID: 'y1',
            borderColor: 'rgba(0, 127, 127, 0.25)',
            backgroundColor: 'rgba(0, 127, 127, 0.25)',
            fill: true,
            stepped: 'before',
            pointRadius: 0,
            borderWidth: 1,
            order: 2
        })
        datasets.push({
            data: remainingStorage,
            label: 'Remaining Storage',
            yAxisID: 'y1',
            borderColor: 'rgba(0, 255, 255, 0.25)',
            backgroundColor: 'rgba(0, 255, 255, 0.25)',
            fill: true,
            stepped: 'before',
            pointRadius: 0,
            borderWidth: 1,
            order: 1
        })
        datasets.push({
            data: uploadPrice,
            label: 'Upload Price',
            yAxisID: 'y',
            borderColor: '#ff0000',
            backgroundColor: 'transparent',
            fill: false,
            stepped: 'before',
            pointRadius: 0,
            borderWidth: 1,
            order: 3
        })
        datasets.push({
            data: downloadPrice,
            label: 'Download Price',
            yAxisID: 'y',
            borderColor: '#0000ff',
            backgroundColor: 'transparent',
            fill: false,
            stepped: 'before',
            pointRadius: 0,
            borderWidth: 1,
            order: 4
        })
        datasets.push({
            data: storagePrice,
            label: 'Storage Price per Month',
            yAxisID: 'y',
            borderColor: props.darkMode ? '#ffffff' : '#000000',
            backgroundColor: 'transparent',
            fill: false,
            stepped: 'before',
            pointRadius: 0,
            borderWidth: 1,
            order: 5
        })
        datasets.push({
            data: collateral,
            label: 'Collateral per Month',
            yAxisID: 'y',
            borderColor: '#00ff00',
            backgroundColor: 'transparent',
            fill: false,
            stepped: 'before',
            pointRadius: 0,
            borderWidth: 1,
            order: 6
        })
        return { labels, datasets }
    }

    const chartRef = useRef<Chart | null>(null)

    const canvasCallback = (canvas: HTMLCanvasElement | null) => {
        if (!canvas) return
        const ctx = canvas.getContext('2d')
        if (ctx) {
            if (chartRef.current) {
                chartRef.current.destroy()
            }
            chartRef.current = new Chart(ctx, {
                type: 'line',
                data: formatData(props.data),
                options: {
                    responsive: true,
                    scales: {
                        x: {
                            grid: {
                                color: props.darkMode ? 'rgba(127, 127, 127, 0.1)': 'rgba(0, 0, 0, 0.1)'
                            }
                        },
                        y: {
                            title: {
                                display: true,
                                text: 'Price in SC/TB'
                            },
                            type: 'linear',
                            position: 'left',
                            beginAtZero: true,
                            grid: {
                                color: props.darkMode ? 'rgba(127, 127, 127, 0.1)': 'rgba(0, 0, 0, 0.1)'
                            }
                        },
                        y1: {
                            title: {
                                display: true,
                                text: 'Storage in TB'
                            },
                            type: 'linear',
                            position: 'right',
                            beginAtZero: true,
                            grid: {
                                drawOnChartArea: false
                            }
                        }
                    },
                    plugins: {
                        legend: {
                            display: true,
                            position: 'bottom'
                        }
                    }
                }
            })
        }
    }

    useEffect(() => {
        if (chartRef.current) {
            chartRef.current.data = formatData(props.data)
            chartRef.current.update()
        }
        // eslint-disable-next-line
    }, [props.data])

    return (
        <canvas ref={canvasCallback}></canvas>
    )
}

export const HostPrices = (props: HostPricesProps) => {
    const [scale, setScale] = useState<ScaleOptions>('day')
    const [maxTimestamp, setMaxTimestamp] = useState((new Date()).getTime())
    const moveLeft = () => {
        if (!props.data || props.data.length === 0) return
        let ts = (new Date(props.data[0].timestamp)).getTime()
        let nts = newTimestamp(props.data, maxTimestamp, scale, false)
        if (nts > ts) {
            setMaxTimestamp(nts)
        }
    }
    const moveRight = () => {
        if (!props.data || props.data.length === 0) return
        let nts = newTimestamp(props.data, maxTimestamp, scale, true)
        if (nts <= (new Date()).getTime()) {
            setMaxTimestamp(nts)
        }
    }
    const zoomIn = () => {
        switch (scale) {
            case 'day':
                break
            case 'week':
                setScale('day')
                break
            case 'month':
                setScale('week')
                break
            case 'year':
                setScale('month')
                break
            default:
        }
    }
    const zoomOut = () => {
        switch (scale) {
            case 'day':
                setScale('week')
                break
            case 'week':
                setScale('month')
                break
            case 'month':
                setScale('year')
                break
            case 'year':
                break
            default:
        }
    }
    return (
        <div className={'host-prices-container' + (props.darkMode ? ' host-prices-dark' : '')}>
            <p>Historic Price Development</p>
            <PriceChart
                data={props.data}
                scale={scale}
                setScale={setScale}
                maxTimestamp={maxTimestamp}
                setMaxTimestamp={setMaxTimestamp}
                darkMode={props.darkMode}
            />
            {props.data &&
                <Controls
                    darkMode={props.darkMode}
                    zoomIn={zoomIn}
                    zoomOut={zoomOut}
                    moveLeft={moveLeft}
                    moveRight={moveRight}
                />
            }
        </div>
    )
}
