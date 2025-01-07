import './CountrySelector.css'
import { countryByCode } from '../../api'

type CountrySelectorProps = {
	options: string[],
	value: string,
	onChange: (value: string) => any,
	darkMode: boolean
}

export const CountrySelector = (props: CountrySelectorProps) => {
	return (
		<div className="country-selector-container">
			<label className={props.darkMode ? 'country-selector-dark' : ''}>
				<span className="country-selector-text">Filter by country:</span>
				<select
					className="country-selector-select"
					value={props.value}
					onChange={(event: React.ChangeEvent<HTMLSelectElement>) => {
						props.onChange(event.target.value)
					}}
					tabIndex={1}
				>
					<option key="country-none" value="">All Countries</option>
					{props.options && props.options
						.sort((a, b) => (countryByCode(a) || '').localeCompare(countryByCode(b) || ''))
						.map(option => (
						<option
							key={'country-' + option}
							value={option}
						>{countryByCode(option)}</option>
					))}
				</select>
			</label>
		</div>
	)
}