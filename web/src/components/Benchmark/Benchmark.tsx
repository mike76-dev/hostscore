import './Benchmark.css'
import { convertSize } from '../../api'

type BenchmarkProps = {
	timestamp: string,
	success: boolean,
    upload: number,
    download: number,
	error: string
}

export const Benchmark = (props: BenchmarkProps) => {
	return (
		<div className="benchmark-container">
			<div className={'benchmark-' + (props.success ? 'pass' : 'fail')}>
				{new Date(props.timestamp).toLocaleString()}
			</div>
			{props.success === false ?
				<div className="benchmark-info">{props.error}</div>
			:
                <div className="benchmark-info">
                    <span>&#8613;</span>
                    <span>{convertSize(props.upload) + '/s'}</span>
                    <span>&nbsp;&nbsp;</span>
                    <span>&#8615;</span>
                    <span>{convertSize(props.download) + '/s'}</span>
                    <span>&nbsp;</span>
                </div>
			}
		</div>
	)
}