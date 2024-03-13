import './index.css'

type HostSearchProps = {
	darkMode: boolean,
	value: string,
	onChange: (value: string) => any
}

const HostSearch = (props: HostSearchProps) => {
	return (
		<div className="host-search-container">
			<label className={props.darkMode ? 'host-search-dark' : ''}>
				<span className="host-search-text">Search a host:</span>
				<input
					className="host-search-input"
					type="text"
					value={props.value}
					onChange={(event: React.ChangeEvent<HTMLInputElement>) => {
						props.onChange(event.target.value)
					}}
					tabIndex={1}
				/>
			</label>
		</div>
	)
}

export default HostSearch