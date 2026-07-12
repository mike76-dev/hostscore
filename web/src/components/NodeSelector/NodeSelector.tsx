import './NodeSelector.css'
import { useLocations } from '../../api'

type NodeSelectorProps = {
	darkMode: boolean,
	node: string,
	setNode: (node: string) => any,
}

export const NodeSelector = (props: NodeSelectorProps) => {
	const locations = useLocations()
	return (
		<div className="seg" role="group" aria-label="Benchmark node">
			<button
				key="global"
				tabIndex={1}
				aria-pressed={props.node === 'global'}
				onClick={() => props.setNode('global')}
			>Global</button>
			{locations.map(location => (
				<button
					key={location.short}
					tabIndex={1}
					aria-pressed={props.node === location.short}
					onClick={() => props.setNode(location.short)}
				>{location.long}</button>
			))}
		</div>
	)
}
