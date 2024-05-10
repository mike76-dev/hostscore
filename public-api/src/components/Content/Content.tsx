import SwaggerUI from 'swagger-ui-react'
import 'swagger-ui-react/swagger-ui.css'
import Spec from '../../openapi.json'
import './Content.css'

type ContentProps = {
	darkMode: boolean,
}

export const Content = (props: ContentProps) => {
	return (
		<div className={'content' + (props.darkMode ? ' content-dark-mode' : '')}>
			<SwaggerUI spec={Spec} />
		</div>
	)
}