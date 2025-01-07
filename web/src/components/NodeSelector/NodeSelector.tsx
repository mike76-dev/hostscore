import './NodeSelector.css'
import { ChangeEvent } from 'react'
import { useLocations } from '../../api'

type NodeSelectorProps = {
	darkMode: boolean,
	node: string,
	setNode: (node: string) => any,
}

export const NodeSelector = (props: NodeSelectorProps) => {
	const locations = useLocations()
	const onChange = (e: ChangeEvent<HTMLSelectElement>): any => {
		props.setNode(e.target.value)
	}
	return (
		<div className={'node-selector-container' + (props.darkMode ? ' node-selector-dark' : '')}>
			<label>
				<span className="node-selector-text">Select node:</span>
				<select
					className="node-selector-select"
					tabIndex={1}
					onChange={onChange}
				>
					<option key="global" value="global">Global</option>
					{locations.map(location => (
						<option
							key={location.short}
							value={location.short}
						>{location.long}</option>
					))}
				</select>
			</label>
		</div>
	)
}
