#!/usr/bin/env python3
import knot.space
import knot.mcp

args = knot.mcp.get_args()
knot.space.write_file(args['space_name'], args['file_path'], args['content'])

knot.mcp.return_object({
    'file_path': args['file_path'],
    'success': True,
    'message': f"Successfully wrote to {args['file_path']}",
    'bytes_written': len(args['content'])
})
