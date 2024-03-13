import './index.css'

type HostSelectorProps = {
	value: string,
	onChange: (value: string) => any,
	darkMode: boolean
}

const HostSelector = (props: HostSelectorProps) => {
	return (
		<div className="host-selector-container">
			<label className={props.darkMode ? 'host-selector-dark' : ''}>
				<span className="host-selector-text">Display:</span>
				<select
					className="host-selector-select"
					value={props.value}
					onChange={(event: React.ChangeEvent<HTMLSelectElement>) => {
						props.onChange(event.target.value)
					}}
					tabIndex={1}
				>
					<option value="online">Online hosts</option>
					<option value="all">All hosts</option>
				</select>
			</label>
		</div>
	)
}

export default HostSelector